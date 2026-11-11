package repair

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/util/paths"
)

// Starting the history again moves the history and the audit log aside and
// deletes nothing, and refuses while history.permanent is on.
func TestResetHistoryMovesBothAsideAndHonoursPermanent(t *testing.T) {
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{p.HistoryDB(), p.AuditLog()} {
		if err := os.WriteFile(f, []byte("record\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC)
	permanent := func(string) string { return "true" }
	if _, err := ResetHistory(p, now, permanent); err != ErrHistoryPermanent {
		t.Fatalf("with history.permanent on: %v, want ErrHistoryPermanent", err)
	}
	if _, err := os.Stat(p.HistoryDB()); err != nil {
		t.Fatal("a refused reset moved the history")
	}
	moved, err := ResetHistory(p, now, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 2 {
		t.Fatalf("moved %v, want the history and the audit log", moved)
	}
	for _, mv := range moved {
		if _, err := os.Stat(mv[0]); !os.IsNotExist(err) {
			t.Errorf("%s is still there", mv[0])
		}
		if b, err := os.ReadFile(mv[1]); err != nil || string(b) != "record\n" {
			t.Errorf("%s was not kept: %v", mv[1], err)
		}
	}
}
