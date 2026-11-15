package bundler

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// destWith is a fresh ~/.ssh and config home holding the given files.
func destWith(t *testing.T, files map[string]string) (ssh, cfg string) {
	t.Helper()
	base := t.TempDir()
	ssh, cfg = filepath.Join(base, ".ssh"), filepath.Join(base, "cfg")
	for rel, c := range files {
		p := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return ssh, cfg
}

// Restoring an older bundle over a key rotated since used to destroy the
// newer key: the prompt promised a snapshot, but snapshots never hold private
// keys, so nothing kept it (review R-BUG-01). Every file a restore replaces
// with different content is kept first; identical ones are left alone.
func TestRestoreKeepsTheFilesItReplaces(t *testing.T) {
	src, srcCfg := writeSrc(t)
	res, err := New(src, srcCfg, fakeCipher{}).Bundle("age1recipient", t.TempDir(), "20260101-000000")
	if err != nil {
		t.Fatal(err)
	}
	ssh, cfg := destWith(t, map[string]string{
		".ssh/profiles/work/work_a-ed25519":     "ROTATED-SINCE\n",      // newer than the bundle's
		".ssh/profiles/work/work_a-ed25519.pub": "ssh-ed25519 AAAA a\n", // identical
		"cfg/inventory.json":                    `{"v":2}`,              // newer record
	})
	kept := filepath.Join(t.TempDir(), "replaced", "20260102-000000")
	rr, err := New(ssh, cfg, fakeCipher{}).WithReplacedDir(kept).Restore(res.AgePath, "", fakeFP)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"ssh/profiles/work/work_a-ed25519": "ROTATED-SINCE\n",
		"config/inventory.json":            `{"v":2}`,
	} {
		got, err := os.ReadFile(filepath.Join(kept, filepath.FromSlash(rel)))
		if err != nil || string(got) != want {
			t.Errorf("kept %s = %q, %v; want %q", rel, got, err, want)
		}
		if !slices.Contains(rr.Replaced, rel) {
			t.Errorf("Replaced %v does not name %s", rr.Replaced, rel)
		}
	}
	if _, err := os.Stat(filepath.Join(kept, "ssh", "profiles", "work", "work_a-ed25519.pub")); err == nil {
		t.Error("an identical file was kept as if it had been replaced")
	}
	// Windows reports 0666 for every file; owner-only is its ACL's job there.
	if fi, err := os.Stat(filepath.Join(kept, "ssh", "profiles", "work", "work_a-ed25519")); err == nil && runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("a kept private key has mode %v, want 0600", fi.Mode().Perm())
	}
	if got, _ := os.ReadFile(filepath.Join(ssh, "profiles", "work", "work_a-ed25519")); string(got) != "PRIV-A\n" {
		t.Errorf("the bundle's key was not laid down: %q", got)
	}
}

// With nowhere to keep them, a restore refuses before replacing anything that
// differs, rather than destroy it.
func TestRestoreWithNowhereToKeepRefusesToReplace(t *testing.T) {
	src, srcCfg := writeSrc(t)
	res, err := New(src, srcCfg, fakeCipher{}).Bundle("age1recipient", t.TempDir(), "20260101-000000")
	if err != nil {
		t.Fatal(err)
	}
	ssh, cfg := destWith(t, map[string]string{".ssh/profiles/work/work_a-ed25519": "ROTATED-SINCE\n"})
	if _, err := New(ssh, cfg, fakeCipher{}).Restore(res.AgePath, "", fakeFP); err == nil {
		t.Fatal("a restore with nowhere to keep a different key replaced it")
	}
	if got, _ := os.ReadFile(filepath.Join(ssh, "profiles", "work", "work_a-ed25519")); string(got) != "ROTATED-SINCE\n" {
		t.Errorf("the refused restore still changed the key: %q", got)
	}
}
