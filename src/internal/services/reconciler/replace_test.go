package reconciler

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
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
	replaced, failed := ReplaceLocally(p, []string{"work/work_gh-ed25519", "work/no-such-key"}, false, false, &out)
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

// A forced rotation's local replacement moves the current key into the one
// old/ slot. When the key already there was never removed from a host, that
// evicts a key that still logs in - the case a rotation refuses without
// --evict-old, and --force went round the refusal straight into this
// (audit A-BUG-01, 2026-11-16). It is refused the same way unless told to
// evict.
func TestReplaceLocallyKeepsAPredecessorThatStillLogsIn(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	m := loadManifest(t)
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	inv := inventory.New()
	ref := manifest.KeyRef{Profile: "work", KeyName: "work_gh-ed25519"}
	if mk, err := New(p, m, inv, false).MintRef(ref, ""); err != nil || mk == nil {
		t.Fatalf("minting the first key: %v", err)
	}
	if err := m.Save(p.Manifest()); err != nil {
		t.Fatal(err)
	}
	if err := inv.Save(p.Inventory()); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if replaced, _ := ReplaceLocally(p, []string{ref.String()}, false, false, &out); len(replaced) != 1 {
		t.Fatalf("the first replacement failed:\n%s", out.String())
	}
	slot := filepath.Join(manifest.ArchiveDir(m, p.SSHDir, ref), ref.KeyName)
	fp, err := New(p, m, inv, false).ks.Fingerprint(slot + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	// The key now in old/ is still authorised on a host.
	if err := (inventory.Retirements{fp: {Key: ref.String(), NotRemoved: []string{"panel"}}}).Save(inventory.RetiredPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
	kept, err := os.ReadFile(slot)
	if err != nil {
		t.Fatal(err)
	}

	out.Reset()
	replaced, failed := ReplaceLocally(p, []string{ref.String()}, false, false, &out)
	if len(replaced) != 0 || len(failed) != 1 {
		t.Fatalf("replaced %v failed %v: a predecessor that still logs in was evicted\n%s", replaced, failed, out.String())
	}
	if !strings.Contains(out.String(), "panel") || !strings.Contains(out.String(), "--evict-old") {
		t.Errorf("the refusal does not name the host and the way past it:\n%s", out.String())
	}
	if got, _ := os.ReadFile(slot); !bytes.Equal(got, kept) {
		t.Error("old/ changed although the replacement was refused")
	}

	out.Reset()
	if replaced, _ := ReplaceLocally(p, []string{ref.String()}, false, true, &out); len(replaced) != 1 {
		t.Fatalf("told to evict, the replacement still failed:\n%s", out.String())
	}
}
