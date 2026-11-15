package migratesvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A migrated home's .env keeps working: its old SSHMGR_ names were copied
// verbatim and then ignored, so the settings silently fell back to their
// defaults (review R-GAP-03). The old home's own .env is left as it was.
func TestMigrateRenamesTheOldSettingNames(t *testing.T) {
	p, cfg, legacy := setup(t)
	mkLegacy(t, legacy, "legacy")
	if err := os.WriteFile(filepath.Join(legacy, ".env"), []byte("SSHMGR_SNAPSHOT_RETAIN=3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Migrate(p, false, "TS"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(cfg, ".env")); !strings.Contains(string(b), "AUTHAMA_SNAPSHOT_RETAIN=3") {
		t.Errorf("the migrated .env still uses the old name:\n%s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(legacy, ".env")); string(b) != "SSHMGR_SNAPSHOT_RETAIN=3\n" {
		t.Errorf("the old home's .env was changed:\n%s", b)
	}
}
