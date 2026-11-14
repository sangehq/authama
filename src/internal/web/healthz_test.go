package web

import (
	"encoding/json"
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

// /healthz?challenge= answers with an HMAC of the challenge under the
// control key, so `authama serve` can tell the server its record names from
// anything else that took the port before it sends that key (review R-SEC-08).
// The proof gives nothing away: it is keyed, and the challenge is the caller's.
func TestHealthzProvesItHoldsTheControlKey(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, Control: "control-key",
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Instance: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	var h struct{ Proof string }
	if err := json.Unmarshal(do(s, httptestGet("/healthz?challenge=abc123")).Body.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	if h.Proof == "" || h.Proof != HealthProof("control-key", "abc123") {
		t.Errorf("proof = %q, want HealthProof(control, challenge)", h.Proof)
	}
	if h.Proof == HealthProof("another-key", "abc123") || h.Proof == HealthProof("control-key", "other") {
		t.Error("the proof does not depend on both the key and the challenge")
	}
	if body := do(s, httptestGet("/healthz")).Body.String(); strings.Contains(body, "proof") {
		t.Errorf("a proof without a challenge: %s", body)
	}
	noKey, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if body := do(noKey, httptestGet("/healthz?challenge=abc123")).Body.String(); strings.Contains(body, "proof") {
		t.Errorf("a server with no control key proved one: %s", body)
	}
}

// Behind a proxy on the same machine every caller arrives from loopback, so
// a forwarded request is answered as a remote one: no pid, build or start
// time (review R-SEC-20).
func TestHealthzHidesItsProcessFromAProxiedCaller(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Instance: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"X-Forwarded-For", "Forwarded", "X-Real-Ip"} {
		r := httptestGet("/healthz")
		r.RemoteAddr = "127.0.0.1:50000"
		r.Header.Set(header, "198.51.100.7")
		if body := do(s, r).Body.String(); strings.Contains(body, `"pid"`) || strings.Contains(body, `"version"`) {
			t.Errorf("with %s: %s", header, body)
		}
	}
}
