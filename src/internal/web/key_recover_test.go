package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// The key page carries `ssh recover`'s break-glass snippet once the key has a
// public half, and the snippet holds that public key and nothing private.
func TestTheKeyPageOffersTheBreakGlassSnippet(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	key := fixtureKey(t, c)
	if body := c.get("/keys/" + key).Body.String(); strings.Contains(body, `id="recover"`) {
		t.Fatal("a key with no public half offers a snippet")
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	ref, err := m.ResolveKeySelector(key)
	if err != nil {
		t.Fatal(err)
	}
	pub := m.KeyPath(p.SSHDir, ref) + ".pub"
	const line = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl test@example"
	if err := os.MkdirAll(filepath.Dir(pub), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pub, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := c.get("/keys/" + key).Body.String()
	for _, want := range []string{`id="recover"`, "authama ssh recover " + key, "KEY=&#39;" + line} {
		if !strings.Contains(body, want) {
			t.Errorf("the key page lacks %q", want)
		}
	}
	if strings.Contains(body, "PRIVATE KEY") {
		t.Error("the key page holds a private key")
	}
}
