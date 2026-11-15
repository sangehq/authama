package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/sangehq/authama/src/internal/services/doctor"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// -update rewrites testdata/*.golden.json from the server as it is. The
// goldens were written by the hand-written JSON-RPC server, before the move to
// the official SDK, so they pin what a client saw then.
var update = flag.Bool("update", false, "rewrite the golden files")

type fakeReads struct{ ops []history.Op }

func (f fakeReads) History(history.Filter) ([]history.Op, error) { return f.ops, nil }
func (f fakeReads) Doctor() (doctor.Report, error)               { return doctor.Report{}, nil }

// fixture is a home with two profiles, three hosts and one private key on
// disk, and a HOME of its own so list_tokens reads nothing of the machine's.
func fixture(t *testing.T) (Server, string) {
	t.Helper()
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(p.Manifest(), `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "simtabi":{"key_scope":"per_service","code":"simbi","category":"orgs","hosts":[
	    {"alias":"simbi-prd-web-00","hostname":"203.0.113.10","user":"ploi"},
	    {"alias":"oribi-web","hostname":"203.0.113.11","user":"ploi"}]},
	  "work":{"key_scope":"per_service","hosts":[{"alias":"gh","hostname":"github.com","user":"git"}]}}}`)
	write(p.Inventory(), `{"version":1,"keys":{}}`)
	write(filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519"), "-----BEGIN OPENSSH PRIVATE KEY-----\n")
	userHome := filepath.Join(base, "home")
	if err := os.MkdirAll(userHome, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	s := Server{Home: p, Version: "test",
		Reads: fakeReads{ops: []history.Op{{OpID: "abcdef1234567890", Command: "host add", Result: "ok"}}}}
	return s, base
}

// wire is one client session over stdio's framing: it writes every line,
// reads one answer per request, keyed by id, then closes stdin. Every line on
// stdout must be a JSON-RPC message.
func wire(t *testing.T, s Server, lines ...string) map[string]map[string]any {
	t.Helper()
	want := 0
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil || m["id"] != nil {
			want++ // a request, or a line that is not JSON, which is answered with id null
		}
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := s.Serve(ctx, inR, outW)
		_ = outW.Close()
		done <- err
	}()
	go func() {
		for _, l := range lines {
			if _, err := io.WriteString(inW, l+"\n"); err != nil {
				return
			}
		}
	}()
	got := map[string]map[string]any{}
	sc := bufio.NewScanner(outR)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for len(got) < want && sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil || m["jsonrpc"] != "2.0" {
			t.Fatalf("stdout holds a line that is not a JSON-RPC message: %q", sc.Text())
		}
		if m["id"] == nil && m["error"] == nil {
			continue // a notification from the server
		}
		got[fmt.Sprint(m["id"])] = m
	}
	_ = inW.Close()
	go func() { _, _ = io.Copy(io.Discard, outR) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the session ended with %v when stdin closed", err)
		}
	case <-ctx.Done():
		t.Fatal("the server did not stop when stdin closed")
	}
	if len(got) != want {
		t.Fatalf("%d answers, want %d: %v", len(got), want, got)
	}
	return got
}

// connect is a client of the official SDK talking to the server over the
// SDK's in-memory transports.
func connect(t *testing.T, s Server) *sdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st, ct := sdk.NewInMemoryTransports()
	if _, err := s.New().Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test-client", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func golden(t *testing.T, name string, got any) any {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		b, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	return want
}

// normal round-trips v through JSON, replacing the run's temporary home with
// <base>, so two runs compare equal.
func normal(t *testing.T, v any, base string) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if base != "" {
		s = strings.ReplaceAll(s, base, "<base>")
	}
	if wd, err := os.Getwd(); err == nil {
		s = strings.ReplaceAll(s, wd, "<cwd>")
	}
	var out any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
const initialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}`

func call(id int, name, args string) string {
	if args == "" {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q}}`, id, name)
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, args)
}

// The session every tool and every refusal goes through. Its answers are
// pinned in testdata/session.golden.json.
var session = []string{
	initialize, initialized,
	call(3, "list_profiles", `{}`),
	call(4, "list_keys", `{}`),
	call(5, "list_keys", `{"profile":"work"}`),
	call(6, "get_key", `{}`),
	call(7, "get_key", `{"ref":"work/work_gh-ed25519"}`),
	call(8, "get_key", `{"ref":"nope/none"}`),
	call(9, "list_hosts", `{}`),
	call(10, "list_hosts", `{"profile":"simtabi"}`),
	call(11, "list_tokens", `{}`),
	call(12, "list_history", `{"limit":5}`),
	call(13, "get_operation", `{"op_id":"abcdef12"}`),
	call(14, "get_operation", `{"op_id":"abc"}`),
	call(15, "get_operation", `{"op_id":"zzzzzzzz"}`),
	call(16, "list_events", `{}`),
	call(17, "list_events", `{"type":"file.","limit":2}`),
	call(18, "doctor", `{}`),
	call(19, "list_profiles", ``),
	call(20, "list_profiles", `{"bogus":1}`),
	call(21, "list_keys", `[1]`),
	call(22, "rotate_everything", `{}`),
	`{"jsonrpc":"2.0","id":23,"method":"tools/call","params":{}}`,
	`{"jsonrpc":"2.0","id":24,"method":"resources/list"}`,
	`{"jsonrpc":"2.0","id":25,"method":"ping"}`,
	`{"jsonrpc":"2.0","id":26,"method":"prompts/list"}`,
	`{"jsonrpc":"2.0","id":27,"method":"foo/bar"}`,
}

// changed names the answers the move to the official SDK changes, with what
// they are now; every other answer must equal the golden.
var changed = map[string]func(t *testing.T, got, want map[string]any){
	// The tools capability is {} where it was {"listChanged":false}: the SDK
	// leaves out a false listChanged, which means the same. The rest of the
	// answer is unchanged.
	"1": func(t *testing.T, got, want map[string]any) {
		g, w := got["result"].(map[string]any), want["result"].(map[string]any)
		if caps := fmt.Sprint(g["capabilities"]); caps != "map[tools:map[]]" {
			t.Errorf("capabilities %s, want only tools, with no list changes", caps)
		}
		g, w = maps.Clone(g), maps.Clone(w)
		delete(g, "capabilities")
		delete(w, "capabilities")
		if !reflect.DeepEqual(g, w) {
			t.Errorf("initialize changed beyond its capabilities:\n got  %v\n want %v", g, w)
		}
	},
	// A method the server does not declare is still -32601; the SDK words it.
	"24": methodNotFound("resources/list"),
	"26": methodNotFound("prompts/list"),
	"27": methodNotFound("foo/bar"),
}

func methodNotFound(method string) func(t *testing.T, got, want map[string]any) {
	return func(t *testing.T, got, want map[string]any) {
		e, ok := got["error"].(map[string]any)
		if !ok || e["code"] != want["error"].(map[string]any)["code"] || e["message"] != fmt.Sprintf("method not found: %q", method) {
			t.Errorf("%s: %v, want -32601, method not found", method, got)
		}
	}
}

// Every tool's answer, and every refusal, is the one the hand-written server
// gave: the same results, the same tool errors, the same JSON-RPC errors.
func TestEveryAnswerIsTheOneClientsHad(t *testing.T) {
	s, base := fixture(t)
	got := wire(t, s, session...)
	norm := map[string]any{}
	for id, m := range got {
		delete(m, "jsonrpc")
		norm[id] = normal(t, m, base)
	}
	want := golden(t, "session.golden.json", norm).(map[string]any)
	if len(want) != len(norm) {
		t.Fatalf("%d answers, the golden has %d", len(norm), len(want))
	}
	for id, w := range want {
		g := norm[id]
		if check, ok := changed[id]; ok {
			check(t, g.(map[string]any), w.(map[string]any))
			continue
		}
		if !reflect.DeepEqual(g, w) {
			gb, _ := json.Marshal(g)
			wb, _ := json.Marshal(w)
			t.Errorf("answer %s changed:\n got  %s\n want %s", id, gb, wb)
		}
	}
	// The fixture must have been read, or the goldens pin empty answers.
	if keys := fmt.Sprint(norm["4"]); !strings.Contains(keys, "work/work_gh-ed25519") || !strings.Contains(fmt.Sprint(norm["9"]), "simbi-prd-web-00") {
		t.Fatalf("the fixture's keys or hosts are missing: %v %v", norm["4"], norm["9"])
	}
}

// The tool list is the same nine tools with the same names, titles,
// descriptions, input schemas and read-only annotations. The golden is sorted
// by name, the order the SDK lists them in.
func TestTheToolListIsTheOneClientsHad(t *testing.T) {
	s, _ := fixture(t)
	got := wire(t, s, initialize, initialized, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := got["2"]["result"].(map[string]any)["tools"].([]any)
	slices.SortFunc(tools, func(a, b any) int {
		return strings.Compare(a.(map[string]any)["name"].(string), b.(map[string]any)["name"].(string))
	})
	want := golden(t, "tools.golden.json", tools)
	if !reflect.DeepEqual(normal(t, tools, ""), want) {
		gb, _ := json.MarshalIndent(tools, "", "  ")
		t.Errorf("the tool list changed:\n%s", gb)
	}
	if len(tools) != 9 {
		t.Fatalf("%d tools, want 9", len(tools))
	}
	for _, x := range tools {
		tl := x.(map[string]any)
		ann := tl["annotations"].(map[string]any)
		if ann["readOnlyHint"] != true || ann["destructiveHint"] != false || ann["idempotentHint"] != true || ann["openWorldHint"] != false {
			t.Errorf("tool %v is not marked read-only: %v", tl["name"], ann)
		}
	}
}

// initialize names the server, gives its instructions, declares tools, and
// gives a client the revision it asks for when the server speaks it, or the
// newest it speaks.
func TestInitializeNegotiatesAndDescribesTheServer(t *testing.T) {
	s, _ := fixture(t)
	init := wire(t, s, initialize)["1"]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" {
		t.Errorf("negotiated %v, want the client's 2025-06-18", init["protocolVersion"])
	}
	info := init["serverInfo"].(map[string]any)
	if info["name"] != "authama" || info["title"] != "authama" || info["version"] != "test" {
		t.Errorf("serverInfo %v", info)
	}
	if !strings.HasPrefix(fmt.Sprint(init["instructions"]), "authama manages SSH keys, hosts and auth tokens. These tools read only") {
		t.Errorf("instructions %v", init["instructions"])
	}
	if _, ok := init["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("the tools capability is not declared")
	}
	for _, v := range Versions {
		line := strings.Replace(initialize, "2025-06-18", v, 1)
		if got := wire(t, s, line)["1"]["result"].(map[string]any)["protocolVersion"]; got != v {
			t.Errorf("asked for %s, negotiated %v", v, got)
		}
	}
	line := strings.Replace(initialize, "2025-06-18", "1999-01-01", 1)
	if got := wire(t, s, line)["1"]["result"].(map[string]any)["protocolVersion"]; got != Versions[0] {
		t.Errorf("an unknown version negotiated %v, want the newest, %s", got, Versions[0])
	}
}

// A client built on the official SDK connects, lists the tools and calls each
// one, and reads its result as text and as structured content.
func TestAnSDKClientCallsEveryTool(t *testing.T) {
	s, _ := fixture(t)
	cs := connect(t, s)
	ctx := context.Background()
	if cs.InitializeResult().ServerInfo.Name != "authama" {
		t.Errorf("server %v", cs.InitializeResult().ServerInfo)
	}
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range list.Tools {
		names = append(names, tl.Name)
		if tl.Annotations == nil || !tl.Annotations.ReadOnlyHint {
			t.Errorf("tool %s is not marked read-only", tl.Name)
		}
	}
	slices.Sort(names)
	want := []string{"doctor", "get_key", "get_operation", "list_events", "list_history", "list_hosts", "list_keys", "list_profiles", "list_tokens"}
	if !slices.Equal(names, want) {
		t.Fatalf("tools %v, want %v", names, want)
	}
	args := map[string]map[string]any{"get_key": {"ref": "work/work_gh-ed25519"}, "get_operation": {"op_id": "abcdef12"}}
	for _, name := range names {
		res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args[name]})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if res.IsError {
			t.Errorf("%s answered a tool error: %v", name, res.Content)
			continue
		}
		text, ok := res.Content[0].(*sdk.TextContent)
		if !ok || !json.Valid([]byte(text.Text)) {
			t.Errorf("%s: the text content is not JSON: %v", name, res.Content[0])
		}
		var structured any
		b, _ := json.Marshal(res.StructuredContent)
		_ = json.Unmarshal(b, &structured)
		var fromText any
		_ = json.Unmarshal([]byte(text.Text), &fromText)
		if !reflect.DeepEqual(structured, fromText) {
			t.Errorf("%s: the structured content is not the text's JSON", name)
		}
	}
	// A refusal is a tool error the model can read, not a protocol error.
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "get_key", Arguments: map[string]any{}})
	if err != nil || !res.IsError {
		t.Errorf("get_key with no ref: %v, %v", res, err)
	}
	if _, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "rotate_everything"}); err == nil {
		t.Error("an unknown tool was not refused")
	}
}

// No tool is named for a change: the server only reads.
func TestNoToolChangesAnything(t *testing.T) {
	s, _ := fixture(t)
	list, err := connect(t, s).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 9 {
		t.Fatalf("%d tools; this test has stopped measuring the list", len(list.Tools))
	}
	for _, tl := range list.Tools {
		if !strings.HasPrefix(tl.Name, "list_") && !strings.HasPrefix(tl.Name, "get_") && tl.Name != "doctor" {
			t.Errorf("tool %s is not a read", tl.Name)
		}
	}
}

// A line that is not JSON, or not JSON-RPC 2.0, ends the session: the SDK's
// stdio transport stops reading at a message it cannot decode, where the
// hand-written server answered -32700 or -32600 and read on. Nothing but
// protocol messages reaches stdout either way.
func TestMalformedLinesEndTheSession(t *testing.T) {
	s, _ := fixture(t)
	for _, bad := range []string{`not json`, `{"jsonrpc":"1.0","id":2,"method":"ping"}`} {
		var out strings.Builder
		in := strings.Join([]string{initialize, initialized, bad, `{"jsonrpc":"2.0","id":3,"method":"ping"}`}, "\n") + "\n"
		if err := s.Serve(context.Background(), strings.NewReader(in), &out); err == nil {
			t.Errorf("%s: the session did not end with an error", bad)
		}
		for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			if l != "" && !strings.HasPrefix(l, `{"jsonrpc":"2.0"`) {
				t.Errorf("%s: stdout holds %q", bad, l)
			}
			if strings.Contains(l, `"id":3`) {
				t.Errorf("%s: the session read on: %s", bad, l)
			}
		}
	}
}

// A client that writes its requests and closes stdin gets every answer: the
// session ends when the last one is written, not when stdin closes.
func TestEveryRequestIsAnsweredBeforeStdinClosing(t *testing.T) {
	s, _ := fixture(t)
	lines := []string{initialize, initialized}
	for id := 2; id < 40; id++ {
		lines = append(lines, call(id, "list_hosts", `{}`))
	}
	var out strings.Builder
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out.String(), "\n"); n != 39 {
		t.Errorf("%d answers, want 39, one per request", n)
	}
}

// A request before initialize is refused, as the SDK refuses it; the
// hand-written server answered it.
func TestARequestBeforeInitializeIsRefused(t *testing.T) {
	s, _ := fixture(t)
	got := wire(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if _, ok := got["2"]["error"]; !ok {
		t.Errorf("tools/list before initialize: %v", got["2"])
	}
}
