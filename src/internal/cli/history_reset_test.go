package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/owner"
)

// history reset needs the typed word, and then moves the history and the audit
// log aside rather than deleting them; history.permanent refuses it outright.
// (e2e plan 2026-11-16: "history reset ... missing".)
func TestHistoryResetNeedsTheWordAndKeepsTheHistory(t *testing.T) {
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	if _, err := os.Stat(p.HistoryDB()); err != nil {
		t.Fatalf("a change recorded no history: %v", err)
	}
	out, err := runErr(t, "history", "reset", "--confirm", "yes")
	if err == nil {
		t.Fatal("history reset went on without the word")
	}
	// A refusal says nothing moved, so it must not print the moves either:
	// it used to list "move ... -> ..." and then refuse (2026-11-25).
	if strings.Contains(out, "move ") {
		t.Errorf("a refused reset printed its moves:\n%s", out)
	}
	if _, err := os.Stat(p.HistoryDB()); err != nil {
		t.Fatalf("a refused reset moved the history: %v", err)
	}

	t.Setenv("AUTHAMA_HISTORY_PERMANENT", "1")
	if _, err := runErr(t, "history", "reset", "--confirm", "history"); err == nil {
		t.Fatal("history reset ran while history.permanent is on")
	}
	t.Setenv("AUTHAMA_HISTORY_PERMANENT", "0")

	run(t, "history", "reset", "--confirm", "history")
	if _, err := os.Stat(p.HistoryDB()); !os.IsNotExist(err) {
		t.Fatalf("the history is still in place (%v)", err)
	}
	for _, f := range []string{p.HistoryDB(), p.AuditLog()} {
		kept, _ := filepath.Glob(f + ".reset-*")
		if len(kept) != 1 {
			t.Errorf("%s kept as %v, want one .reset- copy", filepath.Base(f), kept)
		}
	}
}

// owner status says plainly whether the home is bound, and once bound, where
// the key is kept and whether the seal matches. (e2e plan: "owner status ...
// missing".)
func TestOwnerStatusSaysWhetherTheHomeIsBound(t *testing.T) {
	if _, err := owner.MachineID(); err != nil {
		t.Skipf("no machine id: %v", err)
	}
	editHome(t)
	t.Setenv(owner.StoreEnv, owner.StoreFile)
	run(t, "ssh", "profile", "add", "work")
	if out := run(t, "owner", "status"); !strings.Contains(out, "not bound") {
		t.Errorf("an unbound home's status: %q", out)
	}
	run(t, "owner", "init", "--yes")
	out := run(t, "owner", "status")
	for _, want := range []string{"bound since", "key kept in  file", "the seal matches"} {
		if !strings.Contains(out, want) {
			t.Errorf("a bound home's status misses %q:\n%s", want, out)
		}
	}
}
