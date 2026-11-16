package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/testcred"
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
	key, other := testcred.Secret("control"), testcred.Secret("other")
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, Control: key,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Instance: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	var h struct{ Proof string }
	if err := json.Unmarshal(do(s, httptestGet("/healthz?challenge=abc123")).Body.Bytes(), &h); err != nil {
		t.Fatal(err)
	}
	if h.Proof == "" || h.Proof != HealthProof(key, "abc123") {
		t.Errorf("proof = %q, want HealthProof(control, challenge)", h.Proof)
	}
	if h.Proof == HealthProof(other, "abc123") || h.Proof == HealthProof(key, "other") {
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
// time (review R-SEC-20). The caller here proves the control key too, so the
// header alone is what withholds them (audit A-SEC-09 added the proof).
func TestHealthzHidesItsProcessFromAProxiedCaller(t *testing.T) {
	key := testcred.Secret("control")
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, Control: key,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Instance: "inst-1"})
	if err != nil {
		t.Fatal(err)
	}
	proven := "/healthz?challenge=abc123&caller=" + HealthCallerProof(key, "inst-1", "abc123")
	r := httptestGet(proven)
	r.RemoteAddr = "127.0.0.1:50000"
	if body := do(s, r).Body.String(); !strings.Contains(body, `"pid"`) {
		t.Fatalf("a proven caller on this machine got no pid, so the test cannot tell the cases apart: %s", body)
	}
	for _, header := range []string{"X-Forwarded-For", "Forwarded", "X-Real-Ip"} {
		r := httptestGet(proven)
		r.RemoteAddr = "127.0.0.1:50000"
		r.Header.Set(header, "198.51.100.7")
		if body := do(s, r).Body.String(); strings.Contains(body, `"pid"`) || strings.Contains(body, `"version"`) {
			t.Errorf("with %s: %s", header, body)
		}
	}
}

// /healthz gives its pid, build and start time only to a caller that proves
// it holds the control key, not to anything on this machine: a TCP forwarder
// (ssh -L, socat) adds no forwarding header, so loopback alone said nothing
// about who was asking (audit A-SEC-09). The caller's proof is an HMAC of the
// instance and its own challenge, so it is no use to any other server or
// challenge.
func TestHealthzGivesItsProcessOnlyToAKeyHolder(t *testing.T) {
	key, other := testcred.Secret("control"), testcred.Secret("other")
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, Control: key,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Instance: "inst-1", Version: "v9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, query string
		want        bool
	}{
		{"no challenge", "", false},
		{"a challenge alone", "?challenge=abc123", false},
		{"another key's proof", "?challenge=abc123&caller=" + HealthCallerProof(other, "inst-1", "abc123"), false},
		{"another instance's proof", "?challenge=abc123&caller=" + HealthCallerProof(key, "inst-2", "abc123"), false},
		{"another challenge's proof", "?challenge=abc123&caller=" + HealthCallerProof(key, "inst-1", "zzz"), false},
		{"the key holder", "?challenge=abc123&caller=" + HealthCallerProof(key, "inst-1", "abc123"), true},
	} {
		r := httptestGet("/healthz" + tc.query)
		r.RemoteAddr = "127.0.0.1:50000"
		body := do(s, r).Body.String()
		if got := strings.Contains(body, `"pid"`) && strings.Contains(body, `"v9.9.9"`); got != tc.want {
			t.Errorf("%s: pid and version given = %v, want %v: %s", tc.name, got, tc.want, body)
		}
	}
}
