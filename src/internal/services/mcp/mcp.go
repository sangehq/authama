// Package mcp is authama's Model Context Protocol server: an AI client starts
// `authama mcp` and talks to it over stdio, as the protocol recommends for a
// local server (no port, so only the client that started it can reach it).
//
// Its tools read only, as the API does, through the same dataset and feed:
// profiles, keys, hosts, which tokens are set (never a secret), the history,
// the event feed and doctor. Nothing here changes the home.
//
// The protocol over stdio is JSON-RPC 2.0, one message per line; stdout holds
// nothing but those messages, and anything else goes to stderr. It is written
// on the standard library: the binary links a fixed, short list of modules
// (cmd/authama/layering_test.go), and the protocol's stdio side is small.
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

	"github.com/simtabi/authama/src/internal/services/dataset"
	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/feed"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Versions are the protocol revisions this server speaks, newest first. A
// client asking for one of them gets it; any other gets the newest.
var Versions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

// Reader is what the tools read besides the dataset and the feed: the
// history and doctor, as the web UI reads them.
type Reader interface {
	History(f history.Filter) ([]history.Op, error)
	Doctor() (doctor.Report, error)
}

// Server answers one client over stdio.
type Server struct {
	Home    paths.Paths
	Reads   Reader
	Version string
}

// JSON-RPC error codes.
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeBadParams      = -32602
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads requests from in and writes answers to out until in ends or ctx
// is cancelled. A notification (no id) is never answered.
func (s Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	enc := json.NewEncoder(out) // one message per line: Encode ends each with \n
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		resp, answer := s.handle([]byte(line))
		if answer {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

func (s Server) handle(line []byte) (response, bool) {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "not JSON: " + err.Error()}}, true
	}
	if len(req.ID) == 0 {
		return response{}, false // a notification, such as notifications/initialized
	}
	resp := response{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		resp.Error = &rpcError{codeInvalidRequest, "not a JSON-RPC 2.0 request"}
		return resp, true
	}
	result, err := s.call(req.Method, req.Params)
	var rerr *rpcError
	switch {
	case errors.As(err, &rerr):
		resp.Error = rerr
	case err != nil:
		resp.Error = &rpcError{codeInvalidRequest, err.Error()}
	default:
		resp.Result = result
	}
	return resp, true
}

func (s Server) call(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(params, &p)
		v := Versions[0]
		if slices.Contains(Versions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "authama", "title": "authama", "version": s.Version},
			"instructions": "authama manages SSH keys, hosts and auth tokens. These tools read only: they list profiles, keys, " +
				"hosts, which tokens are set (never a secret), the history of changes, the event feed and doctor's report.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolList()}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeBadParams, "tools/call needs a tool name"}
		}
		t, ok := toolByName(p.Name)
		if !ok {
			return nil, &rpcError{codeBadParams, "no tool " + p.Name}
		}
		args := map[string]any{}
		if len(p.Arguments) > 0 && string(p.Arguments) != "null" {
			if err := json.Unmarshal(p.Arguments, &args); err != nil {
				return nil, &rpcError{codeBadParams, "arguments must be an object"}
			}
		}
		out, err := t.run(s, args)
		if err != nil {
			// A tool's failure is its result, so the model can read it.
			return map[string]any{"content": []map[string]string{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		b, _ := json.Marshal(out)
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(b)}}, "structuredContent": out}, nil
	}
	return nil, &rpcError{codeNoMethod, "no method " + method}
}

func (e *rpcError) Error() string { return e.Message }

// profilesResult is list_profiles' answer.
type profilesResult struct {
	Profiles []dataset.Profile `json:"profiles"`
}

// tool is one read the client can call.
type tool struct {
	Name, Title, Description string
	Props                    map[string]prop
	// run returns a JSON object: a map or a struct.
	run func(Server, map[string]any) (any, error)
}

type prop struct {
	Type, Description string
}

func toolList() []map[string]any {
	var out []map[string]any
	for _, t := range tools {
		props := map[string]any{}
		for name, p := range t.Props {
			props[name] = map[string]string{"type": p.Type, "description": p.Description}
		}
		out = append(out, map[string]any{
			"name": t.Name, "title": t.Title, "description": t.Description,
			"inputSchema": map[string]any{"type": "object", "properties": props, "additionalProperties": false},
			"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
		})
	}
	return out
}

func toolByName(name string) (tool, bool) {
	i := slices.IndexFunc(tools, func(t tool) bool { return t.Name == name })
	if i < 0 {
		return tool{}, false
	}
	return tools[i], true
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

var tools = []tool{
	{Name: "list_profiles", Title: "List profiles", Description: "Every profile: its category, key scope and store, and how many hosts and keys it has.",
		run: func(s Server, _ map[string]any) (any, error) {
			ds, err := s.core()
			if err != nil {
				return nil, err
			}
			return profilesResult{Profiles: ds.Profiles}, nil
		}},
	{Name: "list_keys", Title: "List keys", Description: "Every SSH key, or a profile's: type, fingerprint, when it was made and expires, and whether it is on disk.",
		Props: map[string]prop{"profile": {"string", "only this profile's keys"}},
		run: func(s Server, a map[string]any) (any, error) {
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
		}},
	{Name: "get_key", Title: "Get a key", Description: "One key by its ref (profile/name), with the hosts that use it.",
		Props: map[string]prop{"ref": {"string", "the key's ref, profile/name"}},
		run: func(s Server, a map[string]any) (any, error) {
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
		}},
	{Name: "list_hosts", Title: "List hosts", Description: "Every SSH host, or a profile's: address, user, port, provider, key and status.",
		Props: map[string]prop{"profile": {"string", "only this profile's hosts"}},
		run: func(s Server, a map[string]any) (any, error) {
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
		}},
	{Name: "list_tokens", Title: "List auth tokens", Description: "Which package-manager and git auth tokens are set, and where. Never a token's value.",
		run: func(s Server, _ map[string]any) (any, error) {
			ds, err := s.core(dataset.PartTokens)
			if err != nil {
				return nil, err
			}
			return map[string]any{"tokens": ds.Tokens}, nil
		}},
	{Name: "list_history", Title: "List the history", Description: "Recorded changes, newest first, each with its result and events.",
		Props: map[string]prop{"limit": {"integer", "at most this many, up to 200 (default 20)"}, "since": {"string", "from this date, 2026-11-01"},
			"command": {"string", "this command, or a prefix (\"host\")"}, "host": {"string", "changes to this host"}, "profile": {"string", "changes to this profile"},
			"key": {"string", "changes to this key"}},
		run: func(s Server, a map[string]any) (any, error) {
			ops, err := s.Reads.History(history.Filter{Limit: num(a, "limit", 20, 200), Since: str(a, "since"), Command: str(a, "command"),
				Host: str(a, "host"), Profile: str(a, "profile"), Key: str(a, "key")})
			if err != nil {
				return nil, err
			}
			if ops == nil {
				ops = []history.Op{}
			}
			return map[string]any{"operations": ops}, nil
		}},
	{Name: "get_operation", Title: "Get an operation", Description: "One recorded change by its id (or its first 8 characters), with its events.",
		Props: map[string]prop{"op_id": {"string", "the operation's id"}},
		run: func(s Server, a map[string]any) (any, error) {
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
		}},
	{Name: "list_events", Title: "List events", Description: "The event feed: changes that finished and watched files that changed, oldest first. Pass the returned next to read on.",
		Props: map[string]prop{"after": {"string", "the next value a previous call returned"}, "limit": {"integer", "at most this many, up to 200 (default 50)"},
			"type": {"string", "only types starting with this: operation.finished or file."}},
		run: func(s Server, a map[string]any) (any, error) {
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
		}},
	{Name: "doctor", Title: "Run doctor", Description: "authama's health report for this home: permissions, the agent, known_hosts, old keys, the config.",
		run: func(s Server, _ map[string]any) (any, error) {
			rep, err := s.Reads.Doctor()
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": rep.OK(), "report": rep.Format()}, nil
		}},
}
