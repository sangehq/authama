package cli

import (
	"slices"
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

// A refused command never evicts a real restore point (audit A-BUG-04). The
// snapshot was pruned to the retention count when it was taken, before the
// command validated, so at the cap each refused command pushed the oldest
// real snapshot out even though its own was discarded afterwards. Pruning now
// waits until the command's snapshot is known to be kept.
func TestARefusedCommandAtTheCapEvictsNoRestorePoint(t *testing.T) {
	p := editHome(t)
	t.Setenv("AUTHAMA_SNAPSHOT_RETAIN", "2")
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "reconcile", "--yes", "--no-pin")
	run(t, "ssh", "profile", "add", "spare")
	run(t, "ssh", "profile", "add", "extra")
	kept := snapshots.List(p.SnapshotsDir())
	if len(kept) != 2 {
		t.Fatalf("%d snapshots kept at a retention of 2; the test needs the cap reached", len(kept))
	}
	for i := 0; i < 3; i++ {
		if _, err := runErr(t, "ssh", "profile", "add", "work"); err == nil {
			t.Fatal("adding an existing profile was accepted; the test needs a refusal")
		}
	}
	if got := snapshots.List(p.SnapshotsDir()); !slices.Equal(got, kept) {
		t.Errorf("refused commands changed the restore points:\n got %v\nwant %v", got, kept)
	}
	run(t, "ssh", "profile", "add", "home")
	if got := snapshots.List(p.SnapshotsDir()); len(got) != 2 || got[0] != kept[1] {
		t.Errorf("a successful change did not prune to the cap: %v", got)
	}
}
