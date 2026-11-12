// Package mcp is authama's Model Context Protocol server: an AI client starts
// `authama mcp` and talks to it over stdio, as the protocol recommends for a
// local server (no port, so only the client that started it can reach it).
//
// Its tools read only, as the API does, through the same dataset and feed:
// profiles, keys, hosts, which tokens are set (never a secret), the history,
// the event feed and doctor. Nothing here changes the home.
//
// It is built on the official Go SDK, github.com/modelcontextprotocol/go-sdk,
// the owner's decision of 2026-11-12 for every Go project that serves MCP. The
// SDK owns the protocol: framing, JSON-RPC, initialisation and version
// negotiation. stdout holds nothing but its messages; anything else goes to
// stderr.
//
// The tools are registered with the SDK's low-level AddTool and a written
// input schema, so their schemas, results and refusals are the ones clients
// had from the hand-written server this replaced: arguments are read as the
// tools always read them, not validated against the schema first.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/simtabi/authama/src/internal/services/dataset"
	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/feed"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Versions are the protocol revisions this server speaks, newest first. A
// client asking for one of them gets it; any other gets the newest. The SDK
// negotiates, limited to these.
var Versions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

// methods are the requests the server answers. The SDK answers more (an empty
// resources/list, prompts/list); refuse those as the server always has, so it
// answers only what it declares.
var methods = []string{"initialize", "ping", "tools/list", "tools/call"}

// Reader is what the tools read besides the dataset and the feed: the
// history and doctor, as the web UI reads them.
type Reader interface {
	History(f history.Filter) ([]history.Op, error)
	Doctor() (doctor.Report, error)
}

// Server answers one client.
type Server struct {
	Home    paths.Paths
	Reads   Reader
	Version string
}

const instructions = "authama manages SSH keys, hosts and auth tokens. These tools read only: they list profiles, keys, " +
	"hosts, which tokens are set (never a secret), the history of changes, the event feed and doctor's report."

// readOnly is every tool's annotation: it reads, changes nothing, gives the
// same answer twice and reaches nothing outside the home.
var readOnly = &sdk.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: new(false), IdempotentHint: true, OpenWorldHint: new(false)}

// New is the SDK server with authama's tools.
func (s Server) New() *sdk.Server {
	srv := sdk.NewServer(&sdk.Implementation{Name: "authama", Title: "authama", Version: s.Version}, &sdk.ServerOptions{
		Instructions:              instructions,
		SupportedProtocolVersions: Versions,
		// Only tools, whose list never changes; no logging capability.
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
	})
	srv.AddReceivingMiddleware(refusals)

	srv.AddTool(&sdk.Tool{Name: "list_profiles", Title: "List profiles", Annotations: readOnly,
		Description: "Every profile: its category, key scope and store, and how many hosts and keys it has.",
		InputSchema: schema(nil)}, s.handle(s.listProfiles))
	srv.AddTool(&sdk.Tool{Name: "list_keys", Title: "List keys", Annotations: readOnly,
		Description: "Every SSH key, or a profile's: type, fingerprint, when it was made and expires, and whether it is on disk.",
		InputSchema: schema(map[string]prop{"profile": {"string", "only this profile's keys"}})}, s.handle(s.listKeys))
	srv.AddTool(&sdk.Tool{Name: "get_key", Title: "Get a key", Annotations: readOnly,
		Description: "One key by its ref (profile/name), with the hosts that use it.",
		InputSchema: schema(map[string]prop{"ref": {"string", "the key's ref, profile/name"}})}, s.handle(s.getKey))
	srv.AddTool(&sdk.Tool{Name: "list_hosts", Title: "List hosts", Annotations: readOnly,
		Description: "Every SSH host, or a profile's: address, user, port, provider, key and status.",
		InputSchema: schema(map[string]prop{"profile": {"string", "only this profile's hosts"}})}, s.handle(s.listHosts))
	srv.AddTool(&sdk.Tool{Name: "list_tokens", Title: "List auth tokens", Annotations: readOnly,
		Description: "Which package-manager and git auth tokens are set, and where. Never a token's value.",
		InputSchema: schema(nil)}, s.handle(s.listTokens))
	srv.AddTool(&sdk.Tool{Name: "list_history", Title: "List the history", Annotations: readOnly,
		Description: "Recorded changes, newest first, each with its result and events.",
		InputSchema: schema(map[string]prop{"limit": {"integer", "at most this many, up to 200 (default 20)"}, "since": {"string", "from this date, 2026-11-01"},
			"command": {"string", "this command, or a prefix (\"host\")"}, "host": {"string", "changes to this host"}, "profile": {"string", "changes to this profile"},
			"key": {"string", "changes to this key"}})}, s.handle(s.listHistory))
	srv.AddTool(&sdk.Tool{Name: "get_operation", Title: "Get an operation", Annotations: readOnly,
		Description: "One recorded change by its id (or its first 8 characters), with its events.",
		InputSchema: schema(map[string]prop{"op_id": {"string", "the operation's id"}})}, s.handle(s.getOperation))
	srv.AddTool(&sdk.Tool{Name: "list_events", Title: "List events", Annotations: readOnly,
		Description: "The event feed: changes that finished and watched files that changed, oldest first. Pass the returned next to read on.",
		InputSchema: schema(map[string]prop{"after": {"string", "the next value a previous call returned"}, "limit": {"integer", "at most this many, up to 200 (default 50)"},
			"type": {"string", "only types starting with this, whole or without com.simtabi.authama.: operation.finished, file."}})}, s.handle(s.listEvents))
	srv.AddTool(&sdk.Tool{Name: "doctor", Title: "Run doctor", Annotations: readOnly,
		Description: "authama's health report for this home: permissions, the agent, known_hosts, old keys, the config.",
		InputSchema: schema(nil)}, s.handle(s.doctor))
	return srv
}

// Serve answers one client over in and out with the SDK's stdio framing (its
// IOTransport: one JSON-RPC message per line), until in ends or ctx is
// cancelled. `authama mcp` passes the process's stdin and stdout.
//
// When in ends, the session ends only once every request already read has its
// answer. On its own the SDK cancels requests still in flight when its input
// closes, so a client that writes its requests and closes stdin, as a script
// piping into `authama mcp` does, would lose the last answers.
func (s Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	pr, pw := io.Pipe()
	w := &answered{out: out}
	w.cond = sync.NewCond(&w.mu)
	stop := context.AfterFunc(ctx, w.done)
	defer stop()
	go w.feed(in, pw)
	err := s.New().Run(ctx, &sdk.IOTransport{Reader: pr, Writer: w})
	w.done()
	_ = pr.Close() // unblocks feed if the session ended before in did
	return err
}

// answered passes the server's messages to out and counts its answers, so the
// end of the client's input can wait for them.
type answered struct {
	out io.Writer

	mu             sync.Mutex
	cond           *sync.Cond
	asked, answers int
	over           bool
}

// feed copies in to pw line by line, counting the requests, and closes pw once
// in has ended and every request has been answered, or the session is over.
func (a *answered) feed(in io.Reader, pw *io.PipeWriter) {
	r := bufio.NewReader(in)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if isRequest(line) {
				a.mu.Lock()
				a.asked++
				a.mu.Unlock()
			}
			if _, werr := pw.Write(line); werr != nil {
				return
			}
		}
		if err != nil {
			a.mu.Lock()
			for a.answers < a.asked && !a.over {
				a.cond.Wait()
			}
			a.mu.Unlock()
			if errors.Is(err, io.EOF) {
				err = nil
			}
			_ = pw.CloseWithError(err)
			return
		}
	}
}

// Write takes one message from the SDK, which writes each in one call.
func (a *answered) Write(p []byte) (int, error) {
	n, err := a.out.Write(p)
	if isAnswer(p) {
		a.mu.Lock()
		a.answers++
		a.cond.Broadcast()
		a.mu.Unlock()
	}
	return n, err
}

func (a *answered) Close() error { return nil }

// done ends the wait for answers: the session is over or ctx was cancelled.
func (a *answered) done() {
	a.mu.Lock()
	a.over = true
	a.cond.Broadcast()
	a.mu.Unlock()
}

// envelope is the part of a JSON-RPC message that says what it is: a request
// has an id and a method, an answer an id and no method.
type envelope struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

func isRequest(line []byte) bool {
	var e envelope
	return json.Unmarshal(line, &e) == nil && len(e.ID) > 0 && string(e.ID) != "null" && e.Method != ""
}

func isAnswer(msg []byte) bool {
	var e envelope
	return json.Unmarshal(msg, &e) == nil && len(e.ID) > 0 && e.Method == ""
}

// refusals keeps the refusals clients had. A request for a method the server
// does not declare is refused with -32601, which the SDK words itself ("method
// not found"); a call with no tool name or an unknown one keeps its -32602
// and its words, "tools/call needs a tool name" or "no tool".
func refusals(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if !strings.HasPrefix(method, "notifications/") && !slices.Contains(methods, method) {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
		}
		res, err := next(ctx, method, req)
		call, ok := req.(*sdk.CallToolRequest)
		var rerr *jsonrpc.Error
		if !ok || !errors.As(err, &rerr) || rerr.Code != jsonrpc.CodeInvalidParams || rerr.Message != fmt.Sprintf("unknown tool %q", call.Params.Name) {
			return res, err
		}
		if call.Params.Name == "" {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "tools/call needs a tool name"}
		}
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "no tool " + call.Params.Name}
	}
}

type prop struct {
	Type, Description string
}

// schema is a tool's input schema: an object of these properties and no
// others.
func schema(props map[string]prop) map[string]any {
	ps := map[string]any{}
	for name, p := range props {
		ps[name] = map[string]string{"type": p.Type, "description": p.Description}
	}
	return map[string]any{"type": "object", "properties": ps, "additionalProperties": false}
}

// handle turns a read into the SDK's handler. The arguments are an object or
// absent; a read's failure is its result, marked isError, so the model can
// read it; its answer is JSON as text and as structured content.
func (s Server) handle(read func(map[string]any) (any, error)) sdk.ToolHandler {
	return func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		args := map[string]any{}
		if raw := req.Params.Arguments; len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &args); err != nil {
				return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "arguments must be an object"}
			}
		}
		out, err := read(args)
		if err != nil {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: err.Error()}}, IsError: true}, nil
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, err
		}
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, StructuredContent: out}, nil
	}
}

// profilesResult is list_profiles' answer.
type profilesResult struct {
	Profiles []dataset.Profile `json:"profiles"`
}

func (s Server) listProfiles(map[string]any) (any, error) {
	ds, err := s.core()
	if err != nil {
		return nil, err
	}
	return profilesResult{Profiles: ds.Profiles}, nil
}

func (s Server) listKeys(a map[string]any) (any, error) {
	ds, err := s.core()
	if err != nil {
		return nil, err
	}
	keys := []dataset.Key{}
	for _, k := range ds.Keys {
		if p := str(a, "profile"); p == "" || k.Profile == p {
			keys = append(keys, k)
		}
	}
	return map[string]any{"keys": keys}, nil
}

func (s Server) getKey(a map[string]any) (any, error) {
	ref := str(a, "ref")
	if ref == "" {
		return nil, errors.New("give the key's ref, profile/name (list_keys lists them)")
	}
	ds, err := s.core()
	if err != nil {
		return nil, err
	}
	for _, k := range ds.Keys {
		if k.Ref == ref {
			hosts := []dataset.Host{}
			for _, h := range ds.Hosts {
				if h.KeyRef == ref {
					hosts = append(hosts, h)
				}
			}
			return map[string]any{"key": k, "hosts": hosts}, nil
		}
	}
	return nil, fmt.Errorf("no key %s", ref)
}

func (s Server) listHosts(a map[string]any) (any, error) {
	ds, err := s.core()
	if err != nil {
		return nil, err
	}
	hosts := []dataset.Host{}
	for _, h := range ds.Hosts {
		if p := str(a, "profile"); p == "" || h.Profile == p {
			hosts = append(hosts, h)
		}
	}
	return map[string]any{"hosts": hosts}, nil
}

func (s Server) listTokens(map[string]any) (any, error) {
	ds, err := s.core(dataset.PartTokens)
	if err != nil {
		return nil, err
	}
	return map[string]any{"tokens": ds.Tokens}, nil
}

func (s Server) listHistory(a map[string]any) (any, error) {
	ops, err := s.Reads.History(history.Filter{Limit: num(a, "limit", 20, 200), Since: str(a, "since"), Command: str(a, "command"),
		Host: str(a, "host"), Profile: str(a, "profile"), Key: str(a, "key")})
	if err != nil {
		return nil, err
	}
	if ops == nil {
		ops = []history.Op{}
	}
	return map[string]any{"operations": ops}, nil
}

func (s Server) getOperation(a map[string]any) (any, error) {
	id := str(a, "op_id")
	if len(id) < 8 {
		return nil, errors.New("give the operation's id, at least its first 8 characters")
	}
	ops, err := s.Reads.History(history.Filter{})
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		if strings.HasPrefix(op.OpID, id) {
			return map[string]any{"operation": op}, nil
		}
	}
	return nil, fmt.Errorf("no operation %s", id)
}

func (s Server) listEvents(a map[string]any) (any, error) {
	evs, next, err := feed.List(s.Home, str(a, "after"), num(a, "limit", 50, 200))
	if err != nil {
		return nil, err
	}
	var types []string
	if t := str(a, "type"); t != "" {
		types = []string{t}
	}
	out := []feed.Event{}
	for _, e := range evs {
		if feed.Matches(e, types) {
			out = append(out, e)
		}
	}
	return map[string]any{"events": out, "next": next}, nil
}

func (s Server) doctor(map[string]any) (any, error) {
	rep, err := s.Reads.Doctor()
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": rep.OK(), "report": rep.Format()}, nil
}

func str(args map[string]any, k string) string {
	v, _ := args[k].(string)
	return strings.TrimSpace(v)
}

func num(args map[string]any, k string, def, maxN int) int {
	f, ok := args[k].(float64)
	if !ok || f < 1 {
		return def
	}
	return min(int(f), maxN)
}

func (s Server) core(parts ...string) (*dataset.Dataset, error) {
	if len(parts) == 0 {
		parts = []string{dataset.PartCore}
	}
	return dataset.Build(s.Home, dataset.Options{Parts: parts})
}
