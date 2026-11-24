package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The SSH config page shows the Include tree and the source file of each
// Host, the same as `authama ssh config tree`, and never claims a foreign
// Include is managed.
func TestSSHConfigPageShowsTheIncludeTreeAndSourceFiles(t *testing.T) {
	s, p, _ := newTestServer(t)
	if err := os.MkdirAll(filepath.Join(p.SSHDir, "config.d"), 0o700); err != nil {
		t.Fatal(err)
	}
	foreignDir := filepath.Join(t.TempDir(), "orb", "ssh")
	if err := os.MkdirAll(foreignDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreignDir, "config"), []byte("Host orb\n  HostName 127.0.0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config.d", "work.conf"), []byte("Host workbox\n  HostName work.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := "Include " + foreignDir + "/config\nInclude config.d/*.conf\nInclude missing.conf\nHost laptop\n  HostName laptop.example\n"
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	body := c.get("/ssh-config").Body.String()
	for _, want := range []string{
		`id="include-tree"`,
		"workbox",
		"laptop",
		"orb",
		"Include warnings",
		"missing.conf",
		"not managed",
		"authama ssh config tree",
		`id="host-sources"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/ssh-config missing %q", want)
		}
	}
}
