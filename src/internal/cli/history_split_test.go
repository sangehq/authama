package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sangehq/authama/src/internal/services/history"
)

// The default layout keeps config in ~/.config/authama and state in
// ~/.local/state/authama. A locked operation's events must still reach the
// history there, not only when the two homes are one directory (audit B-01).
func TestEventsAreRecordedWhenConfigAndStateHomesDiffer(t *testing.T) {
	p := editHome(t)
	state := filepath.Join(filepath.Dir(p.ConfigDir), "state")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTHAMA_STATE_DIR", state)
	p.StateRoot = state

	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")

	ops := liveOps(t, p)
	if len(ops) != 2 {
		t.Fatalf("recorded %d operations in %s, want 2", len(ops), p.HistoryDB())
	}
	if hostAdd := ops[0]; len(hostAdd.Events) == 0 || hostAdd.Events[0].Event != "host.add" {
		t.Errorf("host add events = %+v, want the host.add audit event", hostAdd.Events)
	}
	if got := historyOps(t, p, history.Filter{Host: "gh"}); len(got) != 1 {
		t.Errorf("--host gh found %d operations, want 1", len(got))
	}
}
