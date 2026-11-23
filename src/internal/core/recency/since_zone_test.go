package recency

import (
	"testing"
	"time"
)

// A --since date is midnight in the zone the listing shows (B-10).
func TestParseSinceInReadsADateInTheGivenZone(t *testing.T) {
	tokyo := time.FixedZone("JST", 9*3600)
	got, err := ParseSinceIn("2026-11-20", time.Now(), tokyo)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 11, 20, 0, 0, 0, 0, tokyo); !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
