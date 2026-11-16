package vmsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The key line is embedded in a single-quoted script run in the guest. A
// quote in the comment was dropped with the comment, but one in the type or
// the key body survived the trim and closed the quote (audit A-SEC-06,
// 2026-11-16). A line that still holds one is refused.
func TestAPublicKeyLineWithAQuoteNeverReachesTheScript(t *testing.T) {
	dir := t.TempDir()
	for name, line := range map[string]string{
		"type": "ssh-ed25519';touch${IFS}/tmp/owned;' AAAAC3NzaC1lZDI1NTE5AAAAIA comment",
		"body": "ssh-ed25519 AAAA'$(id)' comment",
		"back": `ssh-ed25519 AAAA\ comment`,
	} {
		path := filepath.Join(dir, name+".pub")
		if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readPubLine(path)
		if err == nil && strings.ContainsAny(got, `'\`) {
			t.Errorf("%s: a quote reached the script: %q", name, got)
		}
		if err == nil && name != "back" {
			t.Errorf("%s: a key line with a quote in its type or body was accepted: %q", name, got)
		}
	}
	// A quote in the comment alone is still dropped with the comment.
	path := filepath.Join(dir, "comment.pub")
	if err := os.WriteFile(path, []byte("ssh-ed25519 AAAAC3Nz it's mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPubLine(path); err != nil || got != "ssh-ed25519 AAAAC3Nz" {
		t.Errorf("readPubLine = %q, %v", got, err)
	}
}
