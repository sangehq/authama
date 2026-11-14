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
