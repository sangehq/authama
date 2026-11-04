package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

var secretRefField = regexp.MustCompile(`name="secret_ref" value="([A-Za-z0-9_-]{43})"`)

func tokenSetForm(secret string) url.Values {
	return url.Values{"action": {"token-set"}, "target": {"npm"}, "server": {"registry.npmjs.org"}, "secret": {secret}}
}

// The token form is the one place a secret reaches a page's server. It goes
// into the stash on preview and never into a page: the preview and the
// confirmation carry a reference, the reference works once, for the session
// that made it, and the token lands in the tool's file.
func TestTokenSetKeepsTheSecretOutOfEveryPage(t *testing.T) {
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	npmrc := filepath.Join(t.TempDir(), ".npmrc")
	t.Setenv("NPM_CONFIG_USERCONFIG", npmrc)
	secret := testcred.Secret("npm")
	c := signIn(t, s)

	rec := c.change("/changes/preview", tokenSetForm(secret))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "authama token set --target npm --server registry.npmjs.org") {
		t.Fatalf("preview: %d\n%s", rec.Code, body)
	}
	if strings.Contains(body, secret) {
		t.Fatal("the preview page holds the secret")
	}
	if !strings.Contains(body, "adds the npm token for registry.npmjs.org") {
		t.Errorf("the preview does not say what it writes:\n%s", body)
	}
	if _, err := os.Stat(npmrc); !os.IsNotExist(err) {
		t.Fatalf("the preview wrote the token file: %v", err)
	}
	hash, ref := hashField.FindStringSubmatch(body), secretRefField.FindStringSubmatch(body)
	if hash == nil || ref == nil {
		t.Fatalf("the preview has no hash or no secret reference:\n%s", body)
	}
	confirm := func(cl *client) *http.Response {
		f := url.Values{"action": {"token-set"}, "target": {"npm"}, "server": {"registry.npmjs.org"},
			"secret_ref": {ref[1]}, "hash": {hash[1]}}
		return cl.change("/changes/apply", f).Result()
	}

	// Another session cannot use the reference.
	if res := confirm(signIn(t, s)); res.StatusCode == http.StatusOK {
		t.Fatal("another session confirmed with this session's secret")
	}
	if _, err := os.Stat(npmrc); !os.IsNotExist(err) {
		t.Fatal("another session's confirmation wrote the token")
	}

	rec = c.change("/changes/apply", url.Values{"action": {"token-set"}, "target": {"npm"}, "server": {"registry.npmjs.org"},
		"secret_ref": {ref[1]}, "hash": {hash[1]}})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("apply: %d\n%s", rec.Code, rec.Body.String())
	}
	b, err := os.ReadFile(npmrc)
	if err != nil || !strings.Contains(string(b), secret) {
		t.Fatalf("the token was not written: %v", err)
	}

	// Used once: the stash no longer holds it, and the same reference is
	// refused.
	if _, ok := s.secrets.get(s.sessions.get(c.cookie.Value), ref[1], s.now()); ok {
		t.Error("the stash still holds the secret after the change was made")
	}
	if res := confirm(c); res.StatusCode == http.StatusOK {
		t.Error("the secret reference worked twice")
	}

	// A secret posted anywhere but the token form's preview is refused, and
	// not echoed.
	f := addHost("db")
	f.Set("secret", secret)
	if rec := c.change("/changes/preview", f); rec.Code != http.StatusBadRequest || strings.Contains(rec.Body.String(), secret) {
		t.Errorf("a secret on another form: %d", rec.Code)
	}
}
