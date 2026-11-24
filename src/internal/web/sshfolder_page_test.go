package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Help is a dropdown holding Tool integrations and the SSH folder diagram,
// and the diagram draws ~/.ssh with the shared tools folder and its owners.
func TestHelpOpensTheSSHFolderDiagram(t *testing.T) {
	s, p, _ := newTestServer(t)
	m := `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"category":"orgs","key_scope":"per_service",
	    "keys":[{"name":"git-signing","role":"tool","tool":"git","tool_kind":"git-signing"}],
	    "hosts":[{"alias":"gh","hostname":"github.com","user":"git"}]}}}`
	if err := os.WriteFile(p.Manifest(), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"profiles/tools/git/git-signing", "profiles/tools/git/git-signing.pub"} {
		path := filepath.Join(p.SSHDir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := signIn(t, s)
	home := c.get("/").Body.String()
	i := strings.Index(home, `id="navbar-help"`)
	if i < 0 {
		t.Fatal("Help is not a dropdown")
	}
	menu := home[i:]
	menu = menu[:strings.Index(menu, "</li>")]
	for _, want := range []string{`href="/integrations"`, `href="/ssh-folder"`, "SSH folder diagram"} {
		if !strings.Contains(menu, want) {
			t.Errorf("the Help menu lacks %q", want)
		}
	}
	body := c.get("/ssh-folder").Body.String()
	for _, want := range []string{"~/.ssh/", "└── tools/", "# no config, used directly by tools",
		"git-signing{,.pub}", "# git-signing, owned by orgs/work", "ssh structure diagram"} {
		if !strings.Contains(body, want) {
			t.Errorf("/ssh-folder lacks %q", want)
		}
	}
}
