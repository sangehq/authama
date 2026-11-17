package web

import (
	"net/http"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/apitokens"
)

// A token may hold only apiStreamLimit event streams open at once (audit P-05):
// each stream polls the feed every tick for as long as it lives, so one leaked
// or hostile token could otherwise open them without bound. Another token is
// unaffected, and a stream that ends gives its place back.
func TestATokenHoldsOnlyALimitedNumberOfEventStreams(t *testing.T) {
	s, p := apiServerFor(t, true)
	secret, _, err := apitokens.Create(p, "ci", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < apiStreamLimit; i++ {
		if !s.streams.acquire("ci") {
			t.Fatalf("stream %d of %d refused", i+1, apiStreamLimit)
		}
	}
	rec := apiGet(s, "/api/v1/events/stream", secret)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("stream %d for one token = %d, want 429", apiStreamLimit+1, rec.Code)
	}
	if !s.streams.acquire("other") {
		t.Error("another token's stream was refused")
	}
	s.streams.release("ci")
	if !s.streams.acquire("ci") {
		t.Error("a released place was not given back")
	}
}
