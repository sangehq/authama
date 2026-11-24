package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ssh config tree is read-only: it follows Includes (relative, globs, nested)
// and prints each Host with its source file, without writing anything.
func TestConfigTreeListsIncludesAndHostSources(t *testing.T) {
	p := editHome(t)
	if err := os.MkdirAll(filepath.Join(p.SSHDir, "config.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config.d", "work.conf"),
		[]byte("Host workbox\n  HostName work.example\n  IdentityFile ~/.ssh/id_work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config"),
		[]byte("Include config.d/*.conf\nInclude missing.conf\nHost laptop\n  HostName laptop.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := run(t, "ssh", "config", "tree")
	for _, want := range []string{"workbox", "laptop", "config.d", "missing.conf", "Warnings:"} {
		if !strings.Contains(out, want) {
			t.Errorf("tree output missing %q:\n%s", want, out)
		}
	}

	js := run(t, "ssh", "config", "tree", "--json")
	var payload struct {
		Hosts []struct {
			Alias string `json:"alias"`
			File  string `json:"file"`
			Line  int    `json:"line"`
		} `json:"hosts"`
	}
	if err := json.Unmarshal([]byte(js), &payload); err != nil {
		t.Fatalf("json: %v\n%s", err, js)
	}
	got := map[string]int{}
	for _, h := range payload.Hosts {
		got[h.Alias] = h.Line
	}
	if got["laptop"] == 0 || got["workbox"] == 0 {
		t.Errorf("json hosts = %+v", payload.Hosts)
	}

	// Read-only: the config is unchanged.
	b, err := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if err != nil || !strings.Contains(string(b), "Include missing.conf") {
		t.Fatalf("tree wrote to the config: %v %q", err, b)
	}
}
