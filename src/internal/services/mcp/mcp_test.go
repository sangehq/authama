package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

type fakeReads struct{ ops []history.Op }

func (f fakeReads) History(history.Filter) ([]history.Op, error) { return f.ops, nil }
func (f fakeReads) Doctor() (doctor.Report, error)               { return doctor.Report{}, nil }

func session(t *testing.T, s Server, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("stdout holds a line that is not a JSON-RPC message: %q", l)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// A client that starts authama mcp can initialise, list the tools - every one
// read-only - and call one; a notification gets no answer, and errors are
// JSON-RPC errors or, for a tool, a result marked isError.
func TestAClientInitialisesListsAndCallsReadOnlyTools(t *testing.T) {
	s := Server{Home: paths.Paths{ConfigDir: t.TempDir()}, Version: "test",
		Reads: fakeReads{ops: []history.Op{{OpID: "abcdef1234567890", Command: "host add", Result: "ok"}}}}
	msgs := session(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_history","arguments":{"limit":5}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_operation","arguments":{"op_id":"abcdef12"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_key","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"rotate_everything"}}`,
		`{"jsonrpc":"2.0","id":7,"method":"resources/list"}`,
		`not json`,
		`{"jsonrpc":"2.0","id":8,"method":"ping"}`,
	)
	if len(msgs) != 9 {
		t.Fatalf("%d answers, want 9 (the notification is not answered): %v", len(msgs), msgs)
	}
	init := msgs[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" {
		t.Errorf("negotiated %v, want the client's 2025-06-18", init["protocolVersion"])
	}
	if _, ok := init["capabilities"].(map[string]any)["tools"]; !ok {
		t.Error("the tools capability is not declared")
	}
	list := msgs[1]["result"].(map[string]any)["tools"].([]any)
	if len(list) < 8 {
		t.Fatalf("%d tools", len(list))
	}
	for _, x := range list {
		tl := x.(map[string]any)
		ann := tl["annotations"].(map[string]any)
		if ann["readOnlyHint"] != true || ann["destructiveHint"] != false {
			t.Errorf("tool %v is not marked read-only", tl["name"])
		}
		if tl["inputSchema"].(map[string]any)["type"] != "object" {
			t.Errorf("tool %v has no object input schema", tl["name"])
		}
	}
	hist := msgs[2]["result"].(map[string]any)
	if hist["isError"] == true || !strings.Contains(hist["content"].([]any)[0].(map[string]any)["text"].(string), "host add") {
		t.Errorf("list_history: %v", hist)
	}
	if _, ok := hist["structuredContent"].(map[string]any)["operations"]; !ok {
		t.Errorf("list_history has no structured content: %v", hist)
	}
	if op := msgs[3]["result"].(map[string]any); op["isError"] == true {
		t.Errorf("get_operation by its first 8 characters: %v", op)
	}
	if bad := msgs[4]["result"].(map[string]any); bad["isError"] != true {
		t.Errorf("get_key with no ref is not a tool error: %v", bad)
	}
	for i, code := range map[int]float64{5: -32602, 6: -32601, 7: -32700} {
		e, ok := msgs[i]["error"].(map[string]any)
		if !ok || e["code"] != code {
			t.Errorf("answer %d: %v, want error %v", i, msgs[i], code)
		}
	}
	if _, ok := msgs[8]["result"]; !ok {
		t.Errorf("ping: %v", msgs[8])
	}
	// A version the server does not speak gets its newest.
	msgs = session(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`)
	if v := msgs[0]["result"].(map[string]any)["protocolVersion"]; v != Versions[0] {
		t.Errorf("an unknown version negotiated %v", v)
	}
}

// No tool is named for a change: the server only reads.
func TestNoToolChangesAnything(t *testing.T) {
	for _, tl := range tools {
		if !strings.HasPrefix(tl.Name, "list_") && !strings.HasPrefix(tl.Name, "get_") && tl.Name != "doctor" {
			t.Errorf("tool %s is not a read", tl.Name)
		}
	}
}
