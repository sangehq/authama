package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// vmConfig used whatever start-up last read, even when that was another home's
// authama.json: a test that ran start-up left its (empty) VM config behind,
// and a later test's `vm sync` read it instead of its own file, so
// TestPlannedDevboxKeepsItsKey failed in some orders (F-21, 2026-11-25).
// Start-up's settings are used only for the home they were read from.
func TestVMConfigIgnoresAnotherHomesStartup(t *testing.T) {
	p := editHome(t)
	if err := os.WriteFile(p.SettingsFile(), []byte(`{"version": 1, "devboxes": {"docker": {"profile": "devbox", "key": "docker-tunnel"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	other := paths.Paths{SSHDir: filepath.Join(t.TempDir(), ".ssh"), ConfigDir: t.TempDir()}
	startupMu.Lock()
	saved := lastStart
	lastStart = &startupState{Paths: other, Settings: &settings.File{}}
	startupMu.Unlock()
	t.Cleanup(func() { startupMu.Lock(); lastStart = saved; startupMu.Unlock() })

	if got := vmConfig(p).DevboxAliases(); len(got) != 1 || got[0] != "docker" {
		t.Errorf("devboxes = %v, want this home's [docker]", got)
	}
}
