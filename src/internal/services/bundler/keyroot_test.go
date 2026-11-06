package bundler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A home whose keys live under ~/.ssh/keys bundles them under the canonical
// ssh/profiles/ names, and a restore lays them down under the root the
// bundle's own manifest names, which is where that manifest renders them.
func TestABundleFollowsTheKeyRoot(t *testing.T) {
	base := t.TempDir()
	ssh, cfg := filepath.Join(base, ".ssh"), filepath.Join(base, "cfg")
	mk := func(p, c string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk(filepath.Join(ssh, "keys", "work", "work_a-ed25519"), "PRIV-A\n")
	mk(filepath.Join(ssh, "keys", "work", "work_a-ed25519.pub"), "ssh-ed25519 AAAA a\n")
	mk(filepath.Join(ssh, "profiles", "stray", "k"), "NOT MINE\n") // not the key root: not bundled
	mk(filepath.Join(cfg, "manifest.json"), `{"version":1,"defaults":{"key_root":"keys"}}`)

	res, err := New(ssh, cfg, fakeCipher{}).WithKeyRoot("keys").Bundle("age1recipient", t.TempDir(), "20260101-000000")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(res.Contents, ",")
	if !strings.Contains(got, "ssh/profiles/work/work_a-ed25519") || strings.Contains(got, "stray") || strings.Contains(got, "ssh/keys/") {
		t.Fatalf("contents = %v; want the keys/ tree under canonical ssh/profiles/ names only", res.Contents)
	}

	// A fresh machine: no manifest yet, so the bundler's own root is the
	// default, and the bundle's manifest decides.
	base2 := t.TempDir()
	ssh2, cfg2 := filepath.Join(base2, ".ssh"), filepath.Join(base2, "cfg")
	if _, err := New(ssh2, cfg2, fakeCipher{}).Restore(res.AgePath, "", fakeFP); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(ssh2, "keys", "work", "work_a-ed25519")); err != nil || string(b) != "PRIV-A\n" {
		t.Fatalf("the key was not laid down under keys/: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(ssh2, "profiles")); err == nil {
		t.Error("the restore created ~/.ssh/profiles although the manifest names keys/")
	}
}

// age does not authenticate the sender, so the manifest in a bundle is
// checked like any member: a key root it could not hold refuses the whole
// restore before anything is written.
func TestARestoreRefusesAKeyRootTheManifestCouldNotHold(t *testing.T) {
	for _, bad := range []string{"..", "a/b", ".", "config", "authorized_keys"} {
		t.Run(bad, func(t *testing.T) {
			base := t.TempDir()
			ssh, cfg := filepath.Join(base, ".ssh"), filepath.Join(base, "cfg")
			if err := os.MkdirAll(filepath.Join(ssh, "profiles", "w"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(ssh, "profiles", "w", "k"), []byte("K\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(cfg, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfg, "manifest.json"), []byte(`{"defaults":{"key_root":"`+bad+`"}}`), 0o600); err != nil {
				t.Fatal(err)
			}
			res, err := New(ssh, cfg, fakeCipher{}).Bundle("age1recipient", t.TempDir(), "20260101-000000")
			if err != nil {
				t.Fatal(err)
			}
			base2 := t.TempDir()
			ssh2 := filepath.Join(base2, ".ssh")
			if _, err := New(ssh2, filepath.Join(base2, "cfg"), fakeCipher{}).Restore(res.AgePath, "", fakeFP); err == nil {
				t.Fatalf("a bundle naming key_root %q was restored", bad)
			}
			if entries, _ := os.ReadDir(ssh2); len(entries) > 0 {
				t.Fatalf("a refused restore wrote %d entries into ~/.ssh", len(entries))
			}
		})
	}
}
