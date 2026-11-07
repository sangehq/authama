package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Moving the key root from the Keys page is `ssh key root <dir>`: one name,
// typed twice, never a path.
func TestKeyRootFromThePageIsTheCommand(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	keys := c.get("/keys").Body.String()
	for _, want := range []string{`value="key-root"`, `id="key-root"`, "~/.ssh/profiles/", `name="confirm"`} {
		if !strings.Contains(keys, want) {
			t.Errorf("the Keys page has no %q", want)
		}
	}
	rec := c.change("/changes/preview", url.Values{"action": {"key-root"}, "root": {"keys"}, "confirm": {"keys"}})
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "authama ssh key root keys --yes") ||
		!strings.Contains(body, "moves ~/.ssh/profiles/ to ~/.ssh/keys/") {
		t.Fatalf("%d\n%s", rec.Code, body)
	}
	// Typed differently, the preview says why and offers nothing to make.
	rec = c.change("/changes/preview", url.Values{"action": {"key-root"}, "root": {"keys"}, "confirm": {"key"}})
	if !strings.Contains(rec.Body.String(), "type the new directory name again") {
		t.Errorf("a mistyped confirmation was not refused:\n%s", rec.Body.String())
	}
}

func TestKeyRootTakesOneDirectoryName(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, root := range []string{"", "a/b", "../keys", "/tmp/keys", ".hidden", "known_hosts", "config"} {
		f := url.Values{"action": {"key-root"}, "root": {root}, "confirm": {root}}
		if rec := c.change("/changes/preview", f); rec.Code != http.StatusBadRequest {
			t.Errorf("root %q was accepted: %d", root, rec.Code)
		}
	}
}
