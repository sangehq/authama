package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/apitokens"
)

// A caller that keeps sending bad API tokens is slowed down: after
// apiFailLimit failures within apiFailWindow its requests are answered 429,
// valid token or not, until the window passes (R-SEC-19). Other callers are
// unaffected.
func TestTheAPIThrottlesACallerSendingBadTokens(t *testing.T) {
	s, p := apiServerFor(t, true)
	secret, _, err := apitokens.Create(p, "ci", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	from := func(addr string) func(*http.Request) {
		return func(r *http.Request) { r.RemoteAddr = addr }
	}
	for i := 0; i < apiFailLimit; i++ {
		if rec := apiGet(s, "/api/v1/keys", "ama_wrong", from("192.0.2.7:4000")); rec.Code != http.StatusUnauthorized {
			t.Fatalf("bad token %d = %d, want 401", i+1, rec.Code)
		}
	}
	rec := apiGet(s, "/api/v1/keys", secret, from("192.0.2.7:4001"))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after %d failures = %d, want 429", apiFailLimit, rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("a throttled answer names no Retry-After")
	}
	if rec := apiGet(s, "/api/v1/keys", secret, from("192.0.2.8:4000")); rec.Code != http.StatusOK {
		t.Errorf("another caller was throttled too: %d", rec.Code)
	}
}

// The window passes: a throttled caller is answered again once it has.
func TestTheAPIThrottleLiftsAfterTheWindow(t *testing.T) {
	var l failLimiter
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	for i := 0; i < apiFailLimit; i++ {
		l.fail("192.0.2.7", now)
	}
	if wait := l.blocked("192.0.2.7", now); wait <= 0 {
		t.Fatal("not throttled after the limit")
	}
	if wait := l.blocked("192.0.2.7", now.Add(apiFailWindow)); wait > 0 {
		t.Errorf("still throttled after the window: %s", wait)
	}
}

// Rotating addresses does not escape the throttle: an IPv6 /64 is one
// caller, and every address together has a ceiling (audit A-SEC-10).
func TestTheThrottleCountsAnIPv6BlockAndHasACeiling(t *testing.T) {
	r1, _ := http.NewRequest("GET", "/", nil)
	r1.RemoteAddr = "[2001:db8:1:2::a]:4000"
	r2, _ := http.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "[2001:db8:1:2:ffff::b]:4001"
	if remoteHost(r1) != remoteHost(r2) || remoteHost(r1) != "2001:db8:1:2::/64" {
		t.Fatalf("%s vs %s", remoteHost(r1), remoteHost(r2))
	}
	r3, _ := http.NewRequest("GET", "/", nil)
	r3.RemoteAddr = "[::1]:4000"
	if remoteHost(r3) != "::1" {
		t.Errorf("loopback = %s", remoteHost(r3))
	}

	var l failLimiter
	now := time.Now()
	for i := 0; i < apiFailAll; i++ {
		l.fail("198.51.100."+string(rune('a'+i%26))+string(rune('a'+i/26)), now)
	}
	if l.blocked("203.0.113.9", now) == 0 {
		t.Fatal("the ceiling did not hold a fresh address back")
	}
	if l.blocked("203.0.113.9", now.Add(apiFailWindow+time.Second)) != 0 {
		t.Fatal("the ceiling outlived its window")
	}
}
