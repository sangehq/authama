//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The opener runs in a session of its own, so the hang-up a Terminal window
// sends when the launch in it ends cannot stop it before it reaches the
// browser; and a launch waits for it to finish before exiting.
func TestTheOpenerOutlivesTheTerminalItWasStartedFrom(t *testing.T) {
	if cmd := openerCmd([]string{"open", "x"}); cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatal("the opener is not started in a session of its own")
	}
	dir := t.TempDir()
	done := filepath.Join(dir, "done")
	script := filepath.Join(dir, "opener")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 0.3\necho ok > \"$1\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := browserStart([]string{script, done}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(done); err != nil {
		t.Fatalf("the launch did not wait for the opener: %v", err)
	}
}
