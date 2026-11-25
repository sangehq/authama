package importer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// A replacing import promises to back the manifest and inventory up first.
// BackupTargets swallowed every error, so an import whose backup failed went
// ahead with no copy at all (audit F-20, 2026-11-25).
func TestBackupTargetsReportsAFailedBackup(t *testing.T) {
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	if err := os.MkdirAll(filepath.Dir(p.Manifest()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The state home's .state is a file, so no backup directory can be made in it.
	if err := os.MkdirAll(p.State(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.StateDir(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := BackupTargets(p); err == nil {
		t.Error("BackupTargets reported success with no backup made")
	}

	goodBase := t.TempDir()
	good := paths.Paths{SSHDir: filepath.Join(goodBase, ".ssh"), ConfigDir: filepath.Join(goodBase, "cfg")}
	if err := os.MkdirAll(filepath.Dir(good.Manifest()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good.Manifest(), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := BackupTargets(good); err != nil {
		t.Errorf("a backup that can be made failed: %v", err)
	}
}

// Two replacing imports in the same second shared one backup directory, so the
// second's copy overwrote the first's (review, 2026-11-25). The backups for
// this second and the next are made by hand first, so the test does not depend
// on two calls landing in one second.
func TestBackupTargetsNeverReusesABackupDirectory(t *testing.T) {
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	if err := os.MkdirAll(filepath.Dir(p.Manifest()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var taken []string
	for _, at := range []time.Time{now, now.Add(time.Second)} {
		dir := filepath.Join(p.StateDir(), "import-backup-"+at.Format("20060102-150405"))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		taken = append(taken, dir)
	}
	if err := BackupTargets(p); err != nil {
		t.Fatal(err)
	}
	for _, dir := range taken {
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Errorf("the backup went into %s, an existing backup", filepath.Base(dir))
		}
	}
	all, _ := filepath.Glob(filepath.Join(p.StateDir(), "import-backup-*", "manifest.json"))
	if len(all) != 1 {
		t.Errorf("backups holding the manifest: %v", all)
	}
}
