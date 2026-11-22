//go:build windows

package perms

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The ACL read on a real NTFS file (the windows-latest CI leg): SetPerms
// passes the check, a grant to Everyone fails it, and SetPerms fixes it.
func TestWindowsACLCheckReadsTheRealACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetPerms(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if !PermsOK(path, 0o600) {
		t.Fatalf("after SetPerms: %s", WindowsACLProblem(path))
	}
	// *S-1-1-0 is Everyone by SID, whatever the system language.
	if out, err := exec.Command("icacls", path, "/grant", "*S-1-1-0:R").CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v\n%s", err, out)
	}
	if PermsOK(path, 0o600) {
		t.Fatal("a file Everyone can read passed the owner-only check")
	}
	if why := WindowsACLProblem(path); why == "" {
		t.Error("no reason given")
	}
	// A public mode only needs the file to be there.
	if !PermsOK(path, 0o644) {
		t.Error("a 0644 check failed on an existing file")
	}
	if err := SetPerms(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if !PermsOK(path, 0o600) {
		t.Fatalf("SetPerms did not take Everyone off: %s", WindowsACLProblem(path))
	}
	if PermsOK(filepath.Join(t.TempDir(), "missing"), 0o600) {
		t.Error("a missing file passed")
	}
}
