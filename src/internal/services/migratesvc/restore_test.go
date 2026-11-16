package migratesvc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// `migrate --force` moves the current home aside, then copies the legacy one
// in. When the copy failed, the copy was removed and the moved home left at
// its .replaced- name, so the user had no home and an error that did not say
// where it went (review R-BUG-14). A failed copy puts the home back.
func TestAFailedForcedMigrationPutsTheHomeBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes make the copy fail")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of mode 000")
	}
	p, cfg, legacy := setup(t)
	mkLegacy(t, legacy, "legacy")
	unreadable := filepath.Join(legacy, "inventory.json")
	if err := os.WriteFile(unreadable, []byte(`{}`), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o600) })
	mkLegacy(t, cfg, "current")

	if _, err := Migrate(p, true, "TS"); err == nil {
		t.Fatal("a migration whose copy failed reported success")
	}
	b, err := os.ReadFile(filepath.Join(cfg, "manifest.json"))
	if err != nil || string(b) != "current" {
		t.Fatalf("the current home was not put back: %q, %v", b, err)
	}
	if left, _ := filepath.Glob(cfg + ".replaced-*"); len(left) != 0 {
		t.Errorf("the moved-aside home is still there: %v", left)
	}
	if !strings.HasSuffix(cfg, "authama") {
		t.Fatal("setup changed")
	}
}
