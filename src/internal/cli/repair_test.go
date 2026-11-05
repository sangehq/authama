package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repair --check lists a token file others can read and changes nothing;
// repair --yes makes it owner-only, and a second check is clean.
func TestRepairFindsThenFixes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows protects token files with ACLs; repair checks no mode bits there")
	}
	p := editHome(t)
	home := filepath.Dir(p.SSHDir)
	npmrc := filepath.Join(home, ".npmrc")
	if err := os.WriteFile(npmrc, []byte("registry=https://registry.npmjs.org/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runErr(t, "repair", "--check", "--area", "tokens")
	if err == nil || !strings.Contains(out, ".npmrc") {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if fi, _ := os.Stat(npmrc); fi.Mode().Perm() != 0o644 {
		t.Fatal("--check changed the file")
	}
	run(t, "repair", "--area", "tokens", "--yes")
	if fi, _ := os.Stat(npmrc); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode after repair: %o", fi.Mode().Perm())
	}
	run(t, "repair", "--check", "--area", "tokens")
	if _, err := runErr(t, "repair", "--area", "nonsense"); err == nil {
		t.Fatal("an unknown area was accepted")
	}
}

// reset needs the word, moves the config home aside, and keeps ~/.ssh.
func TestResetNeedsTheWordAndKeepsEverything(t *testing.T) {
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	if _, err := runErr(t, "reset", "--confirm", "yes"); err == nil {
		t.Fatal("reset went on without the word")
	}
	if _, err := os.Stat(p.Manifest()); err != nil {
		t.Fatal("a refused reset moved the home")
	}
	run(t, "reset", "--confirm", "reset")
	if _, err := os.Stat(p.ConfigDir); err == nil {
		t.Fatal("the config home is still in place")
	}
	matches, _ := filepath.Glob(p.ConfigDir + ".reset-*")
	if len(matches) != 1 {
		t.Fatalf("kept homes: %v", matches)
	}
	if _, err := os.Stat(filepath.Join(matches[0], "manifest.json")); err != nil {
		t.Fatalf("the manifest was not kept: %v", err)
	}
}
