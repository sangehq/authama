package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// The host forms carry host add/edit's --key-name, --provider, --token-env
// and --proxy: the preview names them as flags, confirming writes them, and a
// provider, variable or proxy authama does not know is refused before any
// preview.
func TestTheHostFormsCarryTheProviderAndProxyOptions(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	for _, page := range []string{"/hosts", "/hosts/gh"} {
		body := c.get(page).Body.String()
		for _, field := range []string{`name="key_name"`, `name="provider"`, `name="token_env"`, `name="proxy"`, `value="cloudflared"`} {
			if !strings.Contains(body, field) {
				t.Errorf("%s lacks %s", page, field)
			}
		}
	}
	form := func() url.Values {
		return url.Values{"action": {"host-add"}, "profile": {"work"}, "alias": {"edge"}, "hostname": {"edge.internal"},
			"user": {"first.last"}, "port": {"22"}, "key_name": {"edge-key"}, "provider": {"gitea-self-hosted"},
			"token_env": {"GITEA_TOKEN_WORK"}, "proxy": {"cloudflared"}}
	}
	rec := c.change("/changes/preview", form())
	body := rec.Body.String()
	for _, want := range []string{"--key-name=edge-key", "--provider=gitea-self-hosted", "--token-env=GITEA_TOKEN_WORK", "--proxy=cloudflared"} {
		if !strings.Contains(body, want) {
			t.Errorf("the preview lacks %s:\n%.900s", want, body)
		}
	}
	m := hashField.FindStringSubmatch(body)
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("preview: %d\n%.900s", rec.Code, body)
	}
	f := form()
	f.Set("hash", m[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.900s", rec.Code, rec.Body.String())
	}
	b, _ := os.ReadFile(p.Manifest())
	for _, want := range []string{`"key_name": "edge-key"`, `"provider": "gitea-self-hosted"`, `"token_env": "GITEA_TOKEN_WORK"`, `"cloudflared"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the manifest lacks %s:\n%s", want, b)
		}
	}

	// An edit with "off" removes the proxy; empty fields leave the rest.
	edit := url.Values{"action": {"host-edit"}, "profile": {"work"}, "alias": {"edge"}, "proxy": {"off"}}
	rec = c.change("/changes/preview", edit)
	if m = hashField.FindStringSubmatch(rec.Body.String()); m == nil || !strings.Contains(rec.Body.String(), "--proxy=off") {
		t.Fatalf("edit preview:\n%.900s", rec.Body.String())
	}
	edit.Set("hash", m[1])
	if rec := c.change("/changes/apply", edit); rec.Code != http.StatusOK {
		t.Fatalf("edit apply: %d", rec.Code)
	}
	b, _ = os.ReadFile(p.Manifest())
	if strings.Contains(string(b), "cloudflared") || !strings.Contains(string(b), `"provider": "gitea-self-hosted"`) {
		t.Errorf("the edit did not clear only the proxy:\n%s", b)
	}

	for field, bad := range map[string]string{"provider": "nope", "token_env": "A B", "proxy": "socks", "key_name": "../x"} {
		f := form()
		f.Set("alias", "edge2")
		f.Set(field, bad)
		if rec := c.change("/changes/preview", f); strings.Contains(rec.Body.String(), `name="hash"`) {
			t.Errorf("%s %q was offered for confirmation", field, bad)
		}
	}
}
