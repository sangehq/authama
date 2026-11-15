package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Adding a host a hand-written block above the managed one already covers
// says so at once: `host add work api.corp` under `Match host *.corp` resolved
// to the Match block's user, and nothing said why (review R-GAP-02).
func TestHostAddWarnsWhenAForeignBlockShadowsIt(t *testing.T) {
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := "Match host *.corp\n    User corpuser\n\n"
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config"), []byte(foreign), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, "ssh", "profile", "add", "work")
	out := run(t, "ssh", "host", "add", "work", "api.corp", "-H", "api.corp", "-u", "deploy")
	if !strings.Contains(out, "api.corp is shadowed") || !strings.Contains(out, "Match host *.corp") {
		t.Errorf("host add did not warn about the Match block:\n%s", out)
	}
	clean := run(t, "ssh", "host", "add", "work", "web", "-H", "web.example", "-u", "deploy")
	if strings.Contains(clean, "web is shadowed") {
		t.Errorf("a host no foreign block covers was reported shadowed:\n%s", clean)
	}
}
