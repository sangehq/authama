package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/inventory"
)

// Moving the key root takes every key with it in one rename, the inventory
// follows, the config is rendered again to point at the new paths (IdentityFile
// and the category Include alike), the doctor finds nothing missing, and a
// key minted afterwards lands under the new root. Moving back undoes it.
func TestMovingTheKeyRootTakesTheKeysAndTheConfigWithIt(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "profile", "add", "acme", "--category", "clients")
	run(t, "ssh", "host", "add", "acme", "box", "-H", "10.0.0.9", "-u", "deploy")
	run(t, "ssh", "reconcile", "--no-passphrase", "--no-pin", "-y")

	if got := strings.TrimSpace(run(t, "ssh", "key", "root")); got != "~/.ssh/profiles/" {
		t.Fatalf("key root before any move = %q", got)
	}
	work := filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519")
	if _, err := os.Stat(work); err != nil {
		t.Fatalf("reconcile minted nothing under profiles/: %v", err)
	}

	moved := run(t, "ssh", "key", "root", "keys", "--yes")

	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles")); !os.IsNotExist(err) {
		var left []string
		_ = filepath.WalkDir(filepath.Join(p.SSHDir, "profiles"), func(path string, _ os.DirEntry, _ error) error {
			left = append(left, path)
			return nil
		})
		t.Fatalf("~/.ssh/profiles is still there: %v\n%s\nholding %v", err, moved, left)
	}
	for _, rel := range []string{"keys/work/work_gh-ed25519", "keys/work/work_gh-ed25519.pub", "keys/clients/acme"} {
		if _, err := os.Stat(filepath.Join(p.SSHDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("~/.ssh/%s is missing after the move: %v", rel, err)
		}
	}
	if got := strings.TrimSpace(run(t, "ssh", "key", "root")); got != "~/.ssh/keys/" {
		t.Errorf("key root after the move = %q", got)
	}
	cfg, _ := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if !strings.Contains(string(cfg), "IdentityFile ~/.ssh/keys/work/work_gh-ed25519") ||
		!strings.Contains(string(cfg), "Include ~/.ssh/keys/clients/config") {
		t.Fatalf("the config was not rendered for the new root:\n%s", cfg)
	}
	if strings.Contains(string(cfg), "~/.ssh/profiles/") {
		t.Fatalf("the config still names ~/.ssh/profiles/:\n%s", cfg)
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Keys) < 2 {
		t.Fatalf("inventory holds %d records, want at least 2", len(inv.Keys))
	}
	for _, rec := range inv.Keys {
		if !strings.HasPrefix(rec.Path, "~/.ssh/keys/") {
			t.Errorf("an inventory record still points at %s", rec.Path)
		}
	}
	if out, err := runErr(t, "doctor"); err != nil {
		t.Fatalf("doctor fails after the move: %v\n%s", err, out)
	}

	// A key minted now goes under the new root.
	run(t, "ssh", "host", "add", "work", "ci", "-H", "ci.example.com", "-u", "git")
	run(t, "ssh", "reconcile", "--no-passphrase", "--no-pin", "-y")
	if _, err := os.Stat(filepath.Join(p.SSHDir, "keys", "work", "work_ci-ed25519")); err != nil {
		t.Fatalf("a new key was not minted under keys/: %v", err)
	}

	run(t, "ssh", "key", "root", "profiles", "--yes")
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "work", "work_ci-ed25519")); err != nil {
		t.Fatalf("moving back to profiles left the keys behind: %v", err)
	}
	cfg, _ = os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if strings.Contains(string(cfg), "~/.ssh/keys/") {
		t.Fatalf("the config still names ~/.ssh/keys/ after moving back:\n%s", cfg)
	}
}

// A name that is not one plain directory, or that belongs to ssh, is refused
// before anything moves; so is a target that already exists beside the root.
func TestTheKeyRootRefusesWhatWouldClobberOrConfuse(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "reconcile", "--no-passphrase", "--no-pin", "-y")

	for _, bad := range []string{".", "a/b", "..", ".keys", "config", "known_hosts2", "authorized_keys2", "id_rsa", "k.pub"} {
		if _, err := runErr(t, "ssh", "key", "root", bad, "--yes"); err == nil {
			t.Errorf("key root %q was accepted", bad)
		}
	}
	if err := os.MkdirAll(filepath.Join(p.SSHDir, "keys"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr(t, "ssh", "key", "root", "keys", "--yes"); err == nil {
		t.Error("moving onto an existing ~/.ssh/keys beside ~/.ssh/profiles was accepted")
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519")); err != nil {
		t.Fatalf("a refused move touched the keys: %v", err)
	}
	if got := strings.TrimSpace(run(t, "ssh", "key", "root")); got != "~/.ssh/profiles/" {
		t.Errorf("a refused move changed the key root to %q", got)
	}
}

// A key root set by editing the manifest instead of through `ssh key root`
// leaves the keys behind, where no scan looks. The doctor says so, and the
// two commands it names bring them over.
func TestTheDoctorFindsKeysLeftBehindByAHandEdit(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "reconcile", "--no-passphrase", "--no-pin", "-y")

	m, err := os.ReadFile(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	edited := regexp.MustCompile(`"defaults":\s*\{`).ReplaceAllLiteralString(string(m), `"defaults": {"key_root": "keys",`)
	if edited == string(m) {
		t.Fatalf("the manifest has no defaults object to edit:\n%s", m)
	}
	if err := os.WriteFile(p.Manifest(), []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runErr(t, "doctor", "--json")
	if !strings.Contains(out, `"stranded_key_root": "~/.ssh/profiles/ holds`) {
		t.Fatalf("doctor does not report the keys left in profiles/:\n%s", out)
	}

	run(t, "ssh", "key", "root", "profiles", "--yes")
	run(t, "ssh", "key", "root", "keys", "--yes")
	if _, err := os.Stat(filepath.Join(p.SSHDir, "keys", "work", "work_gh-ed25519")); err != nil {
		t.Fatalf("the two commands did not move the keys: %v", err)
	}
	out, _ = runErr(t, "doctor", "--json")
	if !strings.Contains(out, `"stranded_key_root": null`) {
		t.Fatalf("doctor still reports stranded keys after the move:\n%s", out)
	}
}
