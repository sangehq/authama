package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// liveOldHome is forceHome after one forced run, with panel-a's previous key
// in old/ recorded as never removed from panel-a: it still logs in there.
func liveOldHome(t *testing.T) (paths.Paths, func(string) string) {
	t.Helper()
	p, _ := forceHome(t)
	run(t, "ssh", "rotate", "--all", "--force", "--yes", "--no-key-backup")
	key := filepath.Join(p.SSHDir, "profiles", "work", "work_panel-a-ed25519")
	fp, err := keystore.New().Fingerprint(filepath.Join(filepath.Dir(key), "old", filepath.Base(key)) + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	if err := (inventory.Retirements{fp: {Key: "work/work_panel-a-ed25519", NotRemoved: []string{"panel-a"}}}).
		Save(inventory.RetiredPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
	pub := func(n string) string {
		b, _ := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", n+".pub"))
		return string(b)
	}
	return p, pub
}

// A factory reset leaves only the new key: --force evicts the previous key
// by default, so a key whose previous key still logs in no longer fails. A
// panel takes keys by hand, so the run says it could not take it off there.
func TestAForcedRotationEvictsAPreviousKeyThatStillLogsIn(t *testing.T) {
	_, pub := liveOldHome(t)
	before := pub("work_panel-a-ed25519")
	out := run(t, "ssh", "rotate", "--all", "--force", "--yes", "--no-key-backup")
	if pub("work_panel-a-ed25519") == before {
		t.Fatalf("the key was not replaced:\n%s", out)
	}
	if !strings.Contains(out, "could not be taken off panel-a") {
		t.Errorf("the run does not say where the previous key still logs in:\n%s", out)
	}
}

// --keep-old under --force keeps the previous key, so the key is not
// replaced, and the run says why and how past it.
func TestKeepOldUnderForceKeepsThePreviousKeyAndSaysSo(t *testing.T) {
	_, pub := liveOldHome(t)
	before := pub("work_panel-a-ed25519")
	out, err := runErr(t, "ssh", "rotate", "--all", "--force", "--keep-old", "--yes", "--no-key-backup")
	if err == nil {
		t.Fatalf("a run that kept a live previous key passed:\n%s", out)
	}
	if pub("work_panel-a-ed25519") != before {
		t.Error("--keep-old replaced the key, evicting its previous key")
	}
	if !strings.Contains(out, "still authorised on panel-a") || !strings.Contains(out, "--evict-old") {
		t.Errorf("the refusal does not name the host and the way past it:\n%s", out)
	}
	if _, err := runErr(t, "ssh", "rotate", "--all", "--keep-old", "--evict-old", "--yes"); err == nil {
		t.Error("--keep-old with --evict-old was accepted")
	}
}

// A forced dry run previews the eviction rather than reporting a failure the
// forced run would not have; with --keep-old it fails as that run would.
func TestAForcedDryRunPreviewsTheEviction(t *testing.T) {
	_, pub := liveOldHome(t)
	before := pub("work_panel-a-ed25519")
	out := run(t, "ssh", "rotate", "--all", "--check", "--force")
	if !strings.Contains(out, "would evict old from 1 host(s)") {
		t.Errorf("the forced dry run does not preview the eviction:\n%s", out)
	}
	out, err := runErr(t, "ssh", "rotate", "--all", "--check", "--force", "--keep-old")
	if err == nil || !strings.Contains(out, "still logs in on panel-a") {
		t.Errorf("a forced dry run keeping the previous key: %v\n%s", err, out)
	}
	if pub("work_panel-a-ed25519") != before {
		t.Error("a dry run changed a key")
	}
}
