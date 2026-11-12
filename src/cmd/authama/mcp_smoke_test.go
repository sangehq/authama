package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// `authama mcp` is off until mcp.enabled is set, and then answers a client
// over stdio with nothing on stdout but protocol messages: a stray line of
// output there would break every client.
func TestTheMCPServerIsOffByDefaultAndKeepsStdoutClean(t *testing.T) {
	run, _ := authama(t)
	if _, errOut, code := run("", "init"); code != 0 {
		t.Fatalf("init exited %d: %s", code, errOut)
	}
	if out, errOut, code := run("", "mcp"); code == 0 || !strings.Contains(errOut, "mcp.enabled") || out != "" {
		t.Fatalf("with mcp off: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, errOut, code := run("", "settings", "set", "mcp.enabled", "true"); code != 0 {
		t.Fatalf("settings set exited %d: %s", code, errOut)
	}
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_profiles","arguments":{}}}`,
	}, "\n") + "\n"
	out, errOut, code := run(in, "mcp")
	if code != 0 {
		t.Fatalf("mcp exited %d: %s", code, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout has %d lines, want the 2 answers:\n%s", len(lines), out)
	}
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil || m["jsonrpc"] != "2.0" {
			t.Fatalf("stdout line is not a JSON-RPC message: %q", l)
		}
		if _, bad := m["error"]; bad {
			t.Fatalf("an error answer: %s", l)
		}
	}
	if !strings.Contains(lines[1], "profiles") {
		t.Errorf("list_profiles answered %s", lines[1])
	}
}
