package cli

import (
	"testing"

	"github.com/sangehq/authama/src/internal/services/snapshots"
)

// A command refused before it changes anything keeps no snapshot: each typo
// took one, and with ten kept, ten typos pushed every real restore point out
// (review R-BUG-06). A command that changes something keeps its snapshot.
func TestARefusedCommandKeepsNoSnapshot(t *testing.T) {
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "reconcile", "--yes", "--no-pin") // ~/.ssh exists from here, so changes snapshot it
	run(t, "ssh", "profile", "add", "spare")
	before := len(snapshots.List(p.SnapshotsDir()))
	if before == 0 {
		t.Fatal("a successful profile add kept no snapshot; the test cannot tell the cases apart")
	}
	for _, args := range [][]string{
		{"ssh", "host", "add", "work", "bad alias", "-H", "h.example", "-u", "u"},
		{"ssh", "profile", "add", "work"}, // exists already
	} {
		if _, err := runErr(t, args...); err == nil {
			t.Fatalf("%v was accepted; the test needs a refusal", args)
		}
	}
	if got := len(snapshots.List(p.SnapshotsDir())); got != before {
		t.Errorf("refused commands left %d new snapshot(s)", got-before)
	}
	run(t, "ssh", "profile", "add", "home")
	if got := len(snapshots.List(p.SnapshotsDir())); got != before+1 {
		t.Errorf("a successful change kept %d new snapshot(s), want 1", got-before)
	}
}
