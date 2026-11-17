package reconciler

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// evictHome is one shared key used by a server (generic ssh, answered by a
// fake ssh that records each call) and a panel that takes keys by hand,
// rotated once on this computer so a previous key sits in old/, recorded as
// still on both hosts and on one the manifest no longer has.
func evictHome(t *testing.T) (paths.Paths, *manifest.Manifest, manifest.KeyRef, string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the fake ssh is a shell script")
	}
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\necho \"$@\" >> "+calls+"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(`{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "ops":{"key_scope":"shared","key_name":"id_ops","hosts":[
	    {"alias":"box","hostname":"box.example","user":"deploy"},
	    {"alias":"panel","hostname":"ploi.example","user":"x","provider":"ploi"}]}}}`), &m); err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	inv := inventory.New()
	ref := manifest.KeyRef{Profile: "ops", KeyName: "id_ops"}
	if mk, err := New(p, &m, inv, false).MintRef(ref, ""); err != nil || mk == nil {
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
	slot := filepath.Join(manifest.ArchiveDir(&m, p.SSHDir, ref), ref.KeyName)
	fp, err := New(p, &m, inv, false).ks.Fingerprint(slot + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if err := (inventory.Retirements{fp: {Key: ref.String(), NotRemoved: []string{"box", "gone", "panel"},
		ByHand: map[string]string{"panel": "https://ploi.example"}}}).Save(inventory.RetiredPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
	return p, &m, ref, calls
}

// Evicting takes the previous key off every host that still accepts it and
// that can take it off, names the rest, and records both, so the checklist
// lists only where it still logs in.
func TestEvictPredecessorTakesThePreviousKeyOffItsHosts(t *testing.T) {
	p, m, ref, calls := evictHome(t)
	slot := filepath.Join(manifest.ArchiveDir(m, p.SSHDir, ref), ref.KeyName)
	pub, _ := os.ReadFile(slot + ".pub")
	ev, err := EvictPredecessor(p, m, New(p, m, inventory.New(), false).ks, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ev.Removed, []string{"box"}) || !slices.Equal(ev.Still, []string{"gone", "panel"}) {
		t.Fatalf("eviction = %+v, want box removed and gone, panel still", ev)
	}
	log, _ := os.ReadFile(calls)
	if body := strings.Fields(string(pub))[1]; !strings.Contains(string(log), "box.example") || !strings.Contains(string(log), body) {
		t.Errorf("ssh was not asked to remove the previous key from box:\n%s", log)
	}
	all, err := inventory.LoadRetirements(inventory.RetiredPath(p.Inventory()))
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range all {
		if !slices.Equal(rec.Removed, []string{"box"}) || !slices.Equal(rec.NotRemoved, []string{"gone", "panel"}) {
			t.Errorf("the record = %+v", rec)
		}
	}
}

// A forced replacement told to evict takes the previous key off first, says
// where it could not, and replaces the key.
func TestReplaceLocallyEvictsThePreviousKeyWhenTold(t *testing.T) {
	p, _, ref, calls := evictHome(t)
	var out bytes.Buffer
	replaced, failed := ReplaceLocally(p, []string{ref.String()}, false, true, &out)
	if len(replaced) != 1 || len(failed) != 0 {
		t.Fatalf("replaced %v failed %v:\n%s", replaced, failed, out.String())
	}
	for _, want := range []string{"evict ops/id_ops: the previous key was taken off box", "could not be taken off gone, panel"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the output lacks %q:\n%s", want, out.String())
		}
	}
	if log, _ := os.ReadFile(calls); !strings.Contains(string(log), "box.example") {
		t.Error("ssh was never asked to remove the previous key")
	}
}
