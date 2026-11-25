package knownhosts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

func TestPath(t *testing.T) {
	s := New("/h/.ssh")
	if got := s.Path(); got != filepath.Join("/h/.ssh", "known_hosts") {
		t.Errorf("known_hosts path=%q", got)
	}
}

func TestEnsureAndAddDedup(t *testing.T) {
	ssh := t.TempDir()
	s := New(ssh)

	created, err := s.Ensure()
	if err != nil || !created {
		t.Fatalf("ensure: created=%v err=%v", created, err)
	}
	if c2, _ := s.Ensure(); c2 {
		t.Error("second ensure should report not-created")
	}

	n, err := s.Add([]string{"github.com ssh-ed25519 AAAA", "github.com ssh-rsa BBBB"})
	if err != nil || n != 2 {
		t.Fatalf("add: n=%d err=%v", n, err)
	}
	// Re-adding one existing + one new -> only the new is appended.
	n, _ = s.Add([]string{"github.com ssh-ed25519 AAAA", "gitlab.com ssh-ed25519 CCCC"})
	if n != 1 {
		t.Errorf("dedup add n=%d want 1", n)
	}
	body, _ := os.ReadFile(s.Path())
	// Names are hashed on the way in, so the store is checked by matching rather
	// than by grepping for the plaintext host.
	if strings.Contains(string(body), "github.com") {
		t.Errorf("host names should be hashed, not stored in the clear:\n%s", body)
	}
	if n := strings.Count(string(body), "ssh-ed25519 AAAA"); n != 1 {
		t.Errorf("the same key was pinned %d times:\n%s", n, body)
	}
	if !strings.HasSuffix(string(body), "\n") {
		t.Error("known_hosts should end with a newline")
	}
}

func TestHostInKnownHosts(t *testing.T) {
	dir := t.TempDir()
	kh := filepath.Join(dir, "known_hosts")
	_ = os.WriteFile(kh, []byte(
		"# comment\n"+
			"github.com,140.82.0.1 ssh-ed25519 AAAA\n"+
			"[example.com]:2222 ssh-rsa BBBB\n"+
			"@cert-authority ca.example.com ssh-ed25519 CCCC\n"), 0o644)

	for _, tok := range []string{"github.com", "140.82.0.1", "[example.com]:2222", "ca.example.com"} {
		if !HostInKnownHosts(kh, tok) {
			t.Errorf("token %q should be found", tok)
		}
	}
	for _, tok := range []string{"gitlab.com", "example.com"} { // example.com only pinned as [host]:port
		if HostInKnownHosts(kh, tok) {
			t.Errorf("token %q should NOT be found", tok)
		}
	}
	if HostInKnownHosts(filepath.Join(dir, "nope"), "github.com") {
		t.Error("missing file should not match")
	}
}

func TestTargetsDedup(t *testing.T) {
	const mj = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","hosts":[
	    {"alias":"a","hostname":"h.com","user":"u"},
	    {"alias":"b","hostname":"h.com","user":"u"}
	  ]}}}`
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(mj), &m); err != nil {
		t.Fatal(err)
	}
	// Same (profile, hostname, port) -> deduped to one target.
	targets, err := Targets(&m)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Errorf("targets=%d want 1 (deduped by profile/host/port)", len(targets))
	}
	if ProfileOfAlias(&m, "b") != "work" {
		t.Errorf("ProfileOfAlias(b)=%q want work", ProfileOfAlias(&m, "b"))
	}
}

func TestAutoPinDisabledAndAlreadyTrusted(t *testing.T) {
	const mj = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","hosts":[{"alias":"gh","hostname":"github.com","user":"git"}]}}}`
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(mj), &m); err != nil {
		t.Fatal(err)
	}
	ssh := t.TempDir()
	s := New(ssh)

	// Disabled via env -> no pins, no network touched.
	off := func(string) string { return "0" }
	if got := s.AutoPin(&m, nil, off); len(got) != 0 {
		t.Errorf("disabled AutoPin added %v want none", got)
	}

	// Enabled but the host is already trusted -> skipped (no network needed).
	if err := os.WriteFile(s.Path(), []byte("github.com ssh-ed25519 AAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	on := func(string) string { return "" }
	if got := s.AutoPin(&m, nil, on); len(got) != 0 {
		t.Errorf("already-trusted host should not be re-pinned, got %v", got)
	}
}

// A known_hosts that exists but cannot be read is not an empty one: Add used to
// rewrite it with only the new pins, dropping every host already trusted
// (audit F-15, 2026-11-25).
func TestAddRefusesAnUnreadableKnownHosts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode 000 does not stop the owner reading a file on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a mode 000 file")
	}
	s := New(t.TempDir())
	if _, err := s.Add([]string{"github.com ssh-ed25519 AAAA"}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.Path(), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.Path(), 0o600) })
	if _, err := s.Add([]string{"gitlab.com ssh-ed25519 CCCC"}); err == nil {
		t.Error("Add over an unreadable known_hosts succeeded; it must refuse")
	}
	if err := os.Chmod(s.Path(), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("known_hosts changed:\nbefore %q\nafter  %q", before, after)
	}
}

// A known_hosts linked in from a dotfiles repository is written through its
// link, as the ssh config and credential files are: pinning a host used to
// replace the link with a regular file (review, 2026-11-25).
func TestPinningWritesThroughALinkedKnownHosts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	base := t.TempDir()
	ssh := filepath.Join(base, ".ssh")
	real := filepath.Join(base, "dotfiles", "known_hosts")
	for _, d := range []string{ssh, filepath.Dir(real)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(real, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(ssh)
	if err := os.Symlink(real, s.Path()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add([]string{"github.com ssh-ed25519 AAAA"}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(s.Path()); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced by a regular file (%v)", err)
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), "ssh-ed25519 AAAA") {
		t.Errorf("the linked file did not get the pin: %q", b)
	}

	// A link to nothing is refused, not replaced.
	ssh2 := filepath.Join(base, "other")
	if err := os.MkdirAll(ssh2, 0o700); err != nil {
		t.Fatal(err)
	}
	s2 := New(ssh2)
	if err := os.Symlink(filepath.Join(base, "missing"), s2.Path()); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Ensure(); err == nil {
		t.Error("Ensure replaced a dangling known_hosts link")
	}
}
