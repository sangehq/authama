package keysvc

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/recency"
)

// Each recorded key gets its recency from the events beside the inventory and
// the day fields; a key with no record has nothing to judge.
func TestRecencyIsWorkedOutPerKey(t *testing.T) {
	s := fixture(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	s.inv.Note("SHA256:gh", inventory.EventDeployed, now.Add(-2*time.Hour))
	rows, err := s.Rows("")
	if err != nil {
		t.Fatal(err)
	}
	invPath := filepath.Join(t.TempDir(), "inventory.json")
	got := Recency(s.m, s.inv, invPath, rows, recency.DefaultPolicy(), now)
	if len(got) != len(rows) {
		t.Fatalf("%d statuses for %d rows", len(got), len(rows))
	}
	gh := got["work/work_gh-ed25519"]
	if !gh.IsNew || gh.NewBecause != inventory.EventDeployed || gh.Pill.Label != "new" {
		t.Errorf("a key deployed two hours ago is new: %+v", gh)
	}
	if spare := got["work/work_spare-rsa"]; spare.IsNew || spare.Pill.Label != "" || len(spare.Events) != 0 {
		t.Errorf("an unrecorded key has no recency: %+v", spare)
	}
	// revoked.json's day is read when key-events.json has no time for it.
	revs := inventory.Revocations{"SHA256:gh": {Key: "work/work_gh-ed25519", RevokedOn: "2026-11-13"}}
	if err := revs.Save(inventory.RevokedPath(invPath)); err != nil {
		t.Fatal(err)
	}
	if gh := Recency(s.m, s.inv, invPath, rows, recency.DefaultPolicy(), now)["work/work_gh-ed25519"]; gh.Pill.Label != "revoked" {
		t.Errorf("a revoked key's pill: %+v", gh.Pill)
	}
}
