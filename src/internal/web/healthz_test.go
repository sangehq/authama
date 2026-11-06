package web

import (
	"net/http"
	"strings"
	"testing"
)

// /healthz answers without a session, with the app and the instance id and
// nothing else; every other page still needs one, and a foreign Host header
// is still refused.
func TestHealthzSaysOnlyWhichInstanceThisIs(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Instance: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	rec := do(s, httptestGet("/healthz"))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"app":"authama","instance":"inst-1"}` {
		t.Fatalf("/healthz: %d %q", rec.Code, rec.Body.String())
	}
	if rec := do(s, httptestGet("/")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/ without a session: %d", rec.Code)
	}
	r, _ := http.NewRequest(http.MethodGet, "http://evil.example/healthz", nil)
	if rec := do(s, r); rec.Code == http.StatusOK {
		t.Fatal("/healthz answered a foreign Host")
	}
}
