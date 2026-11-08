package configsvc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/renderer"
)

// ssh makes no shared-connection master in a missing directory, so writing a
// config with a multiplexed VM host creates it, owner-only; a dry run does not.
func TestWritingAVMHostCreatesTheSocketDirectory(t *testing.T) {
	old := renderer.Multiplex
	renderer.Multiplex = func() bool { return true }
	t.Cleanup(func() { renderer.Multiplex = old })
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(strings.Replace(manifestJSON, `"hostname": "a.com", "user": "u"}`,
		`"hostname": "a", "user": "u", "vm": {"hypervisor": "lima", "name": "a"}}`, 1)), &m); err != nil {
		t.Fatal(err)
	}
	ssh := t.TempDir()
	dir := filepath.Join(ssh, renderer.ControlDir)
	if _, err := New(ssh, &m, false).Write(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("a dry run created the socket directory")
	}
	if _, err := New(ssh, &m, false).Write(false); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("no socket directory: %v", err)
	}
	if fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/' {
		t.Errorf("socket directory mode %v; others must not reach the sockets", fi.Mode().Perm())
	}
}
