package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Rebuilding public keys from the Keys page is `ssh key regen-pub`: one key,
// one profile's keys or every key, each named, never a path.
func TestRegenPubFromThePageIsTheCommand(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	keys := c.get("/keys").Body.String()
	for _, want := range []string{`value="key-regen-pub"`, `id="regen-pub"`, "Rebuild the .pub"} {
		if !strings.Contains(keys, want) {
			t.Errorf("the Keys page has no %q", want)
		}
	}
	if !strings.Contains(c.get("/keys/work/work_gh-ed25519").Body.String(), `value="key-regen-pub"`) {
		t.Error("a key's page offers no rebuild")
	}
	for _, tc := range []struct {
		form url.Values
		cmd  string
	}{
		{url.Values{"key": {"work/work_gh-ed25519"}}, "authama ssh key regen-pub work/work_gh-ed25519 --yes"},
		{url.Values{"profile": {"work"}}, "authama ssh key regen-pub work --yes"},
		{url.Values{"all": {"1"}}, "authama ssh key regen-pub --all --yes"},
		// The panel keeps its profile when "every key" is ticked.
		{url.Values{"profile": {"work"}, "all": {"1"}}, "authama ssh key regen-pub --all --yes"},
	} {
		tc.form.Set("action", "key-regen-pub")
		rec := c.change("/changes/preview", tc.form)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.cmd) {
			t.Errorf("%v: %d, want %q\n%s", tc.form, rec.Code, tc.cmd, rec.Body.String())
		}
	}
}

func TestRegenPubTakesOneNamedSelection(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, f := range []url.Values{
		{},
		{"key": {"work/work_gh-ed25519"}, "profile": {"work"}},
		{"key": {"work/work_gh-ed25519"}, "all": {"1"}},
		{"key": {"../etc/passwd"}},
		{"key": {"work"}},
		{"profile": {"-oProxyCommand=x"}},
		{"profile": {"../work"}},
	} {
		f.Set("action", "key-regen-pub")
		if rec := c.change("/changes/preview", f); rec.Code != http.StatusBadRequest {
			t.Errorf("%v was accepted: %d", f, rec.Code)
		}
	}
}

// A read-only server offers no rebuild.
func TestReadOnlyKeysPageOffersNoRebuild(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if body := signIn(t, s).get("/keys").Body.String(); strings.Contains(body, "key-regen-pub") {
		t.Error("a read-only Keys page offers a rebuild")
	}
}
