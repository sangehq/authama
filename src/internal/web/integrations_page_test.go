package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/apitokens"
	"github.com/sangehq/authama/src/internal/services/webhooks"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

var (
	apiSecret     = regexp.MustCompile(`ama_[A-Za-z0-9_-]{16,}`)
	webhookSecret = regexp.MustCompile(`whsec_[A-Za-z0-9+/=]{16,}`)
)

// A new API token is shown once, in the answer to the form that made it,
// with no-store caching, and never on the page again; revoking it works.
func TestAnAPITokenIsShownOnceAndThenNever(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if body := c.get("/api-tokens").Body.String(); !strings.Contains(body, "No API tokens yet") || !strings.Contains(body, `action="/api-tokens/create"`) {
		t.Fatalf("empty /api-tokens:\n%.1500s", body)
	}
	rec := c.change("/api-tokens/create", url.Values{"name": {"ci"}, "expires": {"30"}})
	secret := apiSecret.FindString(rec.Body.String())
	if rec.Code != http.StatusOK || secret == "" || !strings.Contains(rec.Body.String(), "shown once") {
		t.Fatalf("create: %d\n%.2000s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("the answer holding the token is cached: %q", cc)
	}
	if _, ok := apitokens.Check(p, secret, apitokens.ScopeRead, time.Now()); !ok {
		t.Fatal("the token shown does not work")
	}
	for i := 0; i < 2; i++ {
		body := c.get("/api-tokens").Body.String()
		if strings.Contains(body, secret) || apiSecret.MatchString(body) {
			t.Fatal("the token was shown again")
		}
		if !strings.Contains(body, ">ci") {
			t.Fatalf("the list misses the token:\n%.1500s", body)
		}
	}
	if bad := c.change("/api-tokens/create", url.Values{"name": {"x"}, "expires": {"never"}}); bad.Code != http.StatusBadRequest {
		t.Errorf("a bad expiry: %d", bad.Code)
	}
	if rec := c.change("/api-tokens/revoke", url.Values{"name": {"ci"}}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Revoked API token ci") {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if _, ok := apitokens.Check(p, secret, apitokens.ScopeRead, time.Now()); ok {
		t.Fatal("a revoked token still works")
	}
}

// A webhook's signing secret is shown once when it is added; the list never
// holds it, and the endpoint can be turned off, on and removed.
func TestAWebhookSecretIsShownOnceAndThenNever(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	rec := c.change("/webhooks/add", url.Values{"name": {"ops"}, "url": {"https://hooks.example.com/authama"}, "events": {"key.rotated, host.added"}})
	secret := webhookSecret.FindString(html.UnescapeString(rec.Body.String()))
	if rec.Code != http.StatusOK || secret == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("add: %d %q\n%.2000s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
	}
	eps, _ := webhooks.List(p)
	if len(eps) != 1 || eps[0].Secret != secret || len(eps[0].Events) != 2 {
		t.Fatalf("stored: %+v", eps)
	}
	body := html.UnescapeString(c.get("/webhooks").Body.String())
	if strings.Contains(body, secret) || webhookSecret.MatchString(body) || !strings.Contains(body, "hooks.example.com") {
		t.Fatalf("the list shows the secret or misses the endpoint:\n%.2000s", body)
	}
	if rec := c.change("/webhooks/add", url.Values{"name": {"lan"}, "url": {"https://192.168.1.4/x"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a private address was taken: %d", rec.Code)
	}
	if rec := c.change("/webhooks/disable", url.Values{"name": {"ops"}}); rec.Code != http.StatusOK {
		t.Fatalf("disable: %d", rec.Code)
	}
	if eps, _ := webhooks.List(p); !eps[0].Disabled {
		t.Fatal("not turned off")
	}
	if rec := c.change("/webhooks/enable", url.Values{"name": {"ops"}}); rec.Code != http.StatusOK || strings.Contains(html.UnescapeString(rec.Body.String()), secret) {
		t.Fatalf("enable: %d", rec.Code)
	}
	if rec := c.change("/webhooks/nope", url.Values{"name": {"ops"}}); rec.Code != http.StatusNotFound {
		t.Errorf("an unknown action: %d", rec.Code)
	}
	if rec := c.change("/webhooks/remove", url.Values{"name": {"ops"}}); rec.Code != http.StatusOK {
		t.Fatalf("remove: %d", rec.Code)
	}
	if eps, _ := webhooks.List(p); len(eps) != 0 {
		t.Fatalf("not removed: %+v", eps)
	}
}

// Past the step-up window, making a token or adding a webhook needs the PIN
// in the form; nothing is made without it.
func TestMakingASecretNeedsThePIN(t *testing.T) {
	s, now := pinServer(t)
	c := signIn(t, s)
	if rec := c.change("/unlock", url.Values{"pin": {testcred.Secret("owner")}, "next": {"/api-tokens"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("unlock: %d", rec.Code)
	}
	*now = now.Add(stepUpWindow + time.Second)
	for path, f := range map[string]url.Values{
		"/api-tokens/create": {"name": {"ci"}},
		"/webhooks/add":      {"name": {"ops"}, "url": {"https://hooks.example.com/x"}},
	} {
		rec := c.change(path, f)
		if rec.Code != http.StatusForbidden || apiSecret.MatchString(rec.Body.String()) || webhookSecret.MatchString(html.UnescapeString(rec.Body.String())) {
			t.Fatalf("%s without the PIN: %d", path, rec.Code)
		}
	}
	p := s.opts.Engine.Home()
	if toks, _ := apitokens.List(p); len(toks) != 0 {
		t.Fatal("a token was made without the PIN")
	}
	rec := c.change("/api-tokens/create", url.Values{"name": {"ci"}, "pin": {testcred.Secret("owner")}})
	if rec.Code != http.StatusOK || !apiSecret.MatchString(rec.Body.String()) {
		t.Fatalf("with the PIN: %d", rec.Code)
	}
}

// A read-only server lists tokens and webhooks but offers no form, and takes
// no post.
func TestReadOnlyIntegrationsOfferNoForm(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	for _, path := range []string{"/api-tokens", "/webhooks"} {
		rec := c.get(path)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `method="post" action="`+path) {
			t.Fatalf("read-only %s: %d", path, rec.Code)
		}
	}
	if rec := c.change("/api-tokens/create", url.Values{"name": {"ci"}}); rec.Code == http.StatusOK {
		t.Fatalf("read-only create: %d", rec.Code)
	}
}
