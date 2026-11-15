package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/apitokens"
	"github.com/sangehq/authama/src/internal/util/paths"
)

func apiServerFor(t *testing.T, on bool) (*Server, paths.Paths) {
	t.Helper()
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil),
		Version: "test", API: on})
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}

func apiGet(s *Server, path, bearer string, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7822"+path, nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	for _, m := range mutate {
		m(r)
	}
	return do(s, r)
}

// The API is off by default, and says how to turn it on, whatever token is
// sent.
func TestTheAPIIsOffByDefault(t *testing.T) {
	s, p := apiServerFor(t, false)
	secret, _, err := apitokens.Create(p, "ci", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec := apiGet(s, "/api/v1/keys", secret)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "api.enabled") {
		t.Fatalf("with the API off: %d %s", rec.Code, rec.Body.String())
	}
}

// On, the API answers only a live token, only reads, and shares nothing with
// the browser's sign-in in either direction.
func TestTheAPIAnswersOnlyAnAPITokenAndOnlyReads(t *testing.T) {
	s, p := apiServerFor(t, true)
	secret, _, err := apitokens.Create(p, "ci", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if rec := apiGet(s, "/api/v1/keys", ""); rec.Code != http.StatusUnauthorized || !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Bearer") {
		t.Errorf("no token: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	if rec := apiGet(s, "/api/v1/keys", apitokens.Prefix+"nope"); rec.Code != http.StatusUnauthorized {
		t.Errorf("a made-up token: %d", rec.Code)
	}
	// The launch token signs a browser in; it is not an API token.
	if rec := apiGet(s, "/api/v1/keys", testToken); rec.Code != http.StatusUnauthorized {
		t.Errorf("the launch token was taken as an API token: %d", rec.Code)
	}
	// A signed-in browser's session cookie does not reach the API.
	c := signIn(t, s)
	if rec := apiGet(s, "/api/v1/keys", "", func(r *http.Request) { r.AddCookie(c.cookie) }); rec.Code != http.StatusUnauthorized {
		t.Errorf("a session cookie reached the API: %d", rec.Code)
	}
	// A page in a browser sends Origin: refused even with a good token.
	if rec := apiGet(s, "/api/v1/keys", secret, func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:7822") }); rec.Code != http.StatusForbidden {
		t.Errorf("a request with Origin: %d", rec.Code)
	}
	// It only reads.
	post := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7822/api/v1/keys", strings.NewReader("{}"))
	post.Header.Set("Authorization", "Bearer "+secret)
	if rec := do(s, post); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("a POST: %d", rec.Code)
	}
	// An API token is not a browser sign-in: the pages refuse it.
	if rec := apiGet(s, "/keys", secret); rec.Code != http.StatusUnauthorized {
		t.Errorf("a page answered an API token: %d", rec.Code)
	}
	// The host check (DNS rebinding) still applies.
	if rec := apiGet(s, "/api/v1/keys", secret, func(r *http.Request) { r.Host = "evil.example:7822" }); rec.Code == http.StatusOK {
		t.Errorf("a foreign Host header was answered: %d", rec.Code)
	}

	rec := apiGet(s, "/api/v1/keys", secret)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("a good token: %d %s", rec.Code, rec.Body.String())
	}
	var keys struct {
		Keys []struct {
			Ref string `json:"ref"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil || len(keys.Keys) == 0 {
		t.Fatalf("keys: %v %s", err, rec.Body.String())
	}
	if rec := apiGet(s, "/api/v1/keys/"+keys.Keys[0].Ref, secret); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"hosts"`) {
		t.Errorf("one key: %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/v1", "/api/v1/profiles", "/api/v1/hosts", "/api/v1/tokens", "/api/v1/history", "/api/v1/events", "/api/v1/doctor"} {
		rec := apiGet(s, path, secret)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		if path == "/api/v1/tokens" && strings.Contains(strings.ToLower(rec.Body.String()), `"secret"`) {
			t.Errorf("the tokens route carries a secret field: %s", rec.Body.String())
		}
	}
	if rec := apiGet(s, "/api/v1/events?after=nonsense", secret); rec.Code != http.StatusBadRequest {
		t.Errorf("a made-up cursor: %d", rec.Code)
	}
	if rec := apiGet(s, "/api/v1/nope", secret); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"code"`) {
		t.Errorf("an unknown route: %d %s", rec.Code, rec.Body.String())
	}

	// Revoked, the token stops working at once.
	if err := apitokens.Revoke(p, "ci"); err != nil {
		t.Fatal(err)
	}
	if rec := apiGet(s, "/api/v1/keys", secret); rec.Code != http.StatusUnauthorized {
		t.Errorf("a revoked token: %d", rec.Code)
	}
}
