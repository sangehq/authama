package reconciler

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// A forced rotation's fallback replaces each named key on this computer: a
// new key in its place, the old one under old/, and a line saying to deploy
// it next. A name that does not resolve fails alone; the rest go on.
func TestReplaceLocallyMintsAfreshAndKeepsTheOldKey(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	m := loadManifest(t)
	ssh := t.TempDir()
	p := paths.Paths{SSHDir: ssh, ConfigDir: t.TempDir()}
	inv := inventory.New()
	ref := manifest.KeyRef{Profile: "work", KeyName: "work_gh-ed25519"}
	mk, err := New(p, m, inv, false).MintRef(ref, "")
	if err != nil || mk == nil {
		t.Fatalf("minting the first key: %v", err)
	}
	if err := m.Save(p.Manifest()); err != nil {
		t.Fatal(err)
	}
	if err := inv.Save(p.Inventory()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(mk.Path)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	replaced, failed := ReplaceLocally(p, []string{"work/work_gh-ed25519", "work/no-such-key"}, false, &out)
	if len(replaced) != 1 || replaced[0] != "work/work_gh-ed25519" {
		t.Errorf("replaced %v, want the one key", replaced)
	}
	if len(failed) != 1 || failed[0] != "work/no-such-key" {
		t.Errorf("failed %v, want the unknown key alone", failed)
	}
	after, err := os.ReadFile(mk.Path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		t.Error("the key was not replaced")
	}
	var kept bool
	_ = filepath.WalkDir(ssh, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.Contains(path, string(filepath.Separator)+"old"+string(filepath.Separator)) {
			if b, _ := os.ReadFile(path); bytes.Equal(b, before) {
				kept = true
			}
		}
		return nil
	})
	if !kept {
		t.Error("the old key was not kept under old/")
	}
	for _, want := range []string{"ok    work/work_gh-ed25519", "next  replaced on this computer", "FAIL  work/no-such-key"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the output lacks %q:\n%s", want, out.String())
		}
	}
}
