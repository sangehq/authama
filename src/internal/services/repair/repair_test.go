package repair

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/owner"
	"github.com/sangehq/authama/src/internal/util/paths"
)

var t0 = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func sandbox(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, paths.DevSSHDir), ConfigDir: filepath.Join(root, paths.DevConfigDir), DevRoot: root}
	for _, d := range []string{p.SSHDir, p.ConfigDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// A corrupt history is renamed aside, never deleted, and a missing state
// directory is made; both are reported first and fixed only by Fix.
func TestRepairMovesACorruptHistoryAsideAndRemakesDirectories(t *testing.T) {
	p := sandbox(t)
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.HistoryDB(), []byte("not a database at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := Home{P: p, Now: func() time.Time { return t0 }}
	found := h.Check(Tool)
	var history, dirs bool
	for _, f := range found {
		history = history || strings.Contains(f.Problem, "history database")
		dirs = dirs || strings.Contains(f.Problem, "is missing")
	}
	if !history || !dirs {
		t.Fatalf("findings: %+v", found)
	}
	if _, err := os.Stat(p.HistoryDB()); err != nil {
		t.Fatal("Check changed something")
	}
	var out bytes.Buffer
	if err := Fix(found, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if _, err := os.Stat(p.HistoryDB() + ".corrupt-20261105-120000"); err != nil {
		t.Fatalf("the corrupt database was not kept: %v", err)
	}
	if _, err := os.Stat(p.StateDir()); err != nil {
		t.Fatalf("the state directory was not made: %v", err)
	}
	if again := h.Check(Tool); len(again) != 0 {
		t.Fatalf("still wrong after Fix: %+v", again)
	}
}

// A token file others can read is made owner-only; an unreadable manifest is
// reported but never touched.
func TestRepairFixesModesAndLeavesWhatItCannotFixAlone(t *testing.T) {
	p := sandbox(t)
	bad := []byte("{ not json")
	if err := os.WriteFile(p.Manifest(), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	found := Home{P: p}.Check(Tool)
	var manifestFinding *Finding
	for i := range found {
		if strings.Contains(found[i].Problem, "manifest") {
			manifestFinding = &found[i]
		}
	}
	if manifestFinding == nil || manifestFinding.Fixable {
		t.Fatalf("the manifest finding: %+v", manifestFinding)
	}
	_ = Fix(found, &bytes.Buffer{})
	if b, _ := os.ReadFile(p.Manifest()); !bytes.Equal(b, bad) {
		t.Fatal("repair rewrote a manifest it could not read")
	}
}

// A factory reset renames the homes aside, keeps ~/.ssh, and unbinds a bound
// home first so a restored one is never bound to a missing key.
func TestFactoryResetMovesAsideAndUnbindsFirst(t *testing.T) {
	if _, err := owner.MachineID(); err != nil {
		t.Skipf("no machine id: %v", err)
	}
	p := sandbox(t)
	if err := os.WriteFile(p.Manifest(), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.SSHDir, "config"), []byte("Host x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Init(p, t0); err != nil {
		t.Fatal(err)
	}
	plan, err := FactoryReset(p, t0)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unbinds || len(plan.Moves) == 0 {
		t.Fatalf("plan: %+v", plan)
	}
	aside := p.ConfigDir + ".reset-20261105-120000"
	if _, err := os.Stat(filepath.Join(aside, "manifest.json")); err != nil {
		t.Fatalf("the manifest was not kept aside: %v", err)
	}
	if _, err := os.Stat(filepath.Join(aside, "owner.json")); err == nil {
		t.Fatal("the kept home is still bound")
	}
	if _, err := os.Stat(p.ConfigDir); err == nil {
		t.Fatal("the config home is still there")
	}
	if b, _ := os.ReadFile(filepath.Join(p.SSHDir, "config")); string(b) != "Host x\n" {
		t.Fatal("~/.ssh was touched")
	}
	if _, err := FactoryReset(p, t0); err == nil {
		t.Fatal("a second reset with nothing to move succeeded")
	}
}
