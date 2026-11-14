package doctor

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// A reconcile killed mid-run leaves a .mint-* directory that can hold a
// private key; doctor said nothing about it until the next reconcile swept it
// (review R-GAP-07). It is now a warning naming the residue and the sweep, so
// `doctor --strict` fails on it.
func TestCrashResidueIsAWarning(t *testing.T) {
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	mint := filepath.Join(p.SSHDir, "profiles", "work", ".mint-123")
	if err := os.MkdirAll(mint, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mint, "key"), []byte("PRIVATE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep := New(p, nil, false).Run(false)
	if !slices.Contains(rep.CrashResidue, "profiles/work/.mint-123") {
		t.Fatalf("CrashResidue = %v", rep.CrashResidue)
	}
	if w := strings.Join(rep.Warnings(), "\n"); !strings.Contains(w, ".mint-123") || !strings.Contains(w, "ssh clean") {
		t.Errorf("the warning does not name the residue and the sweep:\n%s", w)
	}
	if New(p, nil, false).Run(true).OK() {
		t.Error("doctor --strict passed over crash residue")
	}
}
