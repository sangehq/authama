package web

import (
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/structure"
)

// Times on the pages read in the machine's own zone, named, not in UTC.
func TestPageTimesReadInTheLocalZone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no zone database:", err)
	}
	at := time.Date(2026, 11, 22, 20, 10, 5, 0, time.UTC)
	if got, want := whenIn(at, ny), "2026-11-22 15:10 EST"; got != want {
		t.Errorf("whenIn = %q, want %q", got, want)
	}
	if got, want := stampIn(at, ny), "2026-11-22 15:10:05 EST"; got != want {
		t.Errorf("stampIn = %q, want %q", got, want)
	}
}

// The Structure page's Watching card says when it last checked in local time.
func TestStructureWatchingSaysTheLastCheckInLocalTime(t *testing.T) {
	s, p, _ := newTestServer(t)
	looseKey(t, p.SSHDir, "id_stray")
	checked := time.Date(2026, 11, 22, 20, 10, 0, 0, time.UTC)
	if _, _, err := structure.Check(p, checked); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	body := c.get("/structure").Body.String()
	i := strings.Index(body, `id="structure-watch"`)
	if i < 0 {
		t.Fatal("no Watching card")
	}
	if want := "(last check " + whenIn(checked, time.Local) + ")"; !strings.Contains(body[i:], want) {
		t.Errorf("Watching does not say %q:\n%.600s", want, body[i:])
	}
}
