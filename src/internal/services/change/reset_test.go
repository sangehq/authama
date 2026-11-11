package change

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Resetting the settings to their defaults keeps the old file beside the new
// one, so the reset can be undone by renaming it back.
func TestSettingsResetKeepsTheOldFile(t *testing.T) {
	p := home(t)
	const mine = `{"theme":"dark"}`
	if err := os.WriteFile(p.SettingsFile(), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SettingsInit(false).Run(p, io.Discard); err == nil {
		t.Fatal("settings init without force replaced an existing file")
	}
	var out strings.Builder
	if err := SettingsInit(true).Run(p, &out); err != nil {
		t.Fatal(err)
	}
	kept, _ := filepath.Glob(p.SettingsFile() + ".before-reset-*")
	if len(kept) != 1 {
		t.Fatalf("kept copies = %v, want one", kept)
	}
	if b, _ := os.ReadFile(kept[0]); string(b) != mine {
		t.Errorf("the kept copy holds %q, want the old settings", b)
	}
	if b, _ := os.ReadFile(p.SettingsFile()); string(b) == mine {
		t.Error("authama.json was not written again at its defaults")
	}
	if !strings.Contains(out.String(), "kept the old settings") {
		t.Errorf("the output does not say where the old settings went: %q", out.String())
	}
}

// Initialising the home again asks for the word "init", and backs up the
// manifest, inventory and .env before writing fresh ones. ~/.ssh stays.
func TestHomeInitNeedsTheWordAndBacksUpFirst(t *testing.T) {
	p := home(t)
	before, _ := os.ReadFile(p.Manifest())
	if err := HomeInit(false).Run(p, io.Discard); err == nil || !strings.Contains(err.Error(), `"init"`) {
		t.Fatalf("an unconfirmed init = %v, want it refused", err)
	}
	if b, _ := os.ReadFile(p.Manifest()); string(b) != string(before) {
		t.Fatal("an unconfirmed init changed the manifest")
	}
	if err := HomeInit(true).Run(p, io.Discard); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(filepath.Join(p.StateDir(), "init-backup-*", "*"))
	var saved bool
	for _, b := range backups {
		if data, _ := os.ReadFile(b); string(data) == string(before) {
			saved = true
		}
	}
	if !saved {
		t.Errorf("the old manifest is not in the backup: %v", backups)
	}
	if b, _ := os.ReadFile(p.Manifest()); string(b) == string(before) {
		t.Error("the manifest was not written again")
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519")); err != nil {
		t.Errorf("a key in ~/.ssh went: %v", err)
	}
}
