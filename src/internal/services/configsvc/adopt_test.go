package configsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A home onboarded before import adopted its blocks has the user's Host
// blocks above the managed block, defining each host twice (review R-BUG-09).
// config adopt removes the ones the managed block reproduces, keeps one that
// differs, and a second run finds nothing.
func TestAdoptMigratesAHomeImportedTheOldWay(t *testing.T) {
	const foreign = "# mine\nHost srv\n    HostName new.example\n    User deploy\n\n" +
		"Host db\n    HostName old.example\n    User deploy\n\nHost *.lab\n    User labber\n\n"
	svc, ssh := writeWithPreamble(t, foreign)
	cfg := filepath.Join(ssh, "config")
	if c := mustCheck(t, svc).Collisions; len(c) != 2 {
		t.Fatalf("collisions before = %+v, want srv and db", c)
	}
	before, _ := os.ReadFile(cfg)

	dry, err := svc.Adopt(true, filepath.Join(t.TempDir(), "unused"))
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(cfg); string(after) != string(before) {
		t.Fatal("a dry run edited the config")
	}
	if len(dry.Moved) != 1 || dry.Moved[0].Alias != "srv" || !strings.Contains(dry.Diff, "-Host srv") {
		t.Errorf("dry run: moved=%+v diff=\n%s", dry.Moved, dry.Diff)
	}
	if len(dry.Kept) != 1 || !strings.Contains(dry.Kept[0], "old.example") {
		t.Errorf("kept = %v, want db and the HostName that differs", dry.Kept)
	}

	backup := filepath.Join(t.TempDir(), "adopt")
	res, err := svc.Adopt(false, backup)
	if err != nil {
		t.Fatal(err)
	}
	if saved, _ := os.ReadFile(res.Backup); string(saved) != string(before) {
		t.Error("the backup is not the config as it was")
	}
	after, _ := os.ReadFile(cfg)
	for _, kept := range []string{"# mine", "Host db\n    HostName old.example", "Host *.lab"} {
		if !strings.Contains(string(after), kept) {
			t.Errorf("adopt lost %q:\n%s", kept, after)
		}
	}
	chk := mustCheck(t, svc)
	if len(chk.Collisions) != 1 || chk.Collisions[0].Alias != "db" || !chk.BlockInSync() {
		t.Errorf("after adopt: collisions=%+v block in sync=%v", chk.Collisions, chk.BlockInSync())
	}
	again, err := svc.Adopt(false, filepath.Join(t.TempDir(), "again"))
	if err != nil {
		t.Fatal(err)
	}
	if third, _ := os.ReadFile(cfg); len(again.Moved) != 0 || string(third) != string(after) {
		t.Errorf("a second adopt changed something: moved=%+v", again.Moved)
	}
}

// Removing a block above the managed one would let a later foreign block that
// also applies supply its value first, so ssh would resolve something else:
// that block is kept.
func TestAdoptKeepsABlockALaterForeignBlockWouldReplace(t *testing.T) {
	svc, _ := writeWithPreamble(t, "Host srv\n    User deploy\n\nHost *\n    User other\n\n")
	res, err := svc.Adopt(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 0 || len(res.Kept) != 1 || !strings.Contains(res.Kept[0], "would apply in its place") {
		t.Errorf("moved=%+v kept=%v", res.Moved, res.Kept)
	}
}

// Below the managed block a single value is never read (the managed host set
// it first), so a stale duplicate there is removed even when it differs.
func TestAdoptRemovesADeadDuplicateBelowTheBlock(t *testing.T) {
	ssh := t.TempDir()
	svc := New(ssh, load(t, shadowManifest), false)
	if _, err := svc.Write(false); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(ssh, "config")
	body, _ := os.ReadFile(cfg)
	if err := os.WriteFile(cfg, append(body, []byte("\nHost srv\n    HostName old.example\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Adopt(false, filepath.Join(t.TempDir(), "b"))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 1 || !mustCheck(t, svc).InSync() {
		t.Errorf("moved=%+v kept=%v", res.Moved, res.Kept)
	}
}
