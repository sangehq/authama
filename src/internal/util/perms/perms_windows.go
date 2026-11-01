//go:build windows

package perms

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
)

// SetPerms restricts path to the current user (the Windows ACL equivalent of
// 600/700). The commands and their order come from icaclsCommands in
// windows_acl.go, which is compiled everywhere and tested there; this file is
// only the part that runs them.
func SetPerms(path string, _ os.FileMode) error {
	if _, err := exec.LookPath("icacls"); err != nil {
		return fmt.Errorf("icacls not found: icacls ships with Windows; ensure it's on PATH")
	}
	owner := aclOwner(os.Getenv, user.Current)
	if owner == "" {
		return fmt.Errorf("cannot determine the current user to grant %s to", path)
	}
	required, optional := icaclsCommands(path, owner)
	if f := os.Getenv("SSHMGR_DIAG_ICACLS_LOG"); f != "" { // TEMPORARY diagnostics
		if fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = fh.WriteString(path + "\n")
			_ = fh.Close()
		}
	}
	for _, argv := range required {
		if err := exec.Command(argv[0], argv[1:]...).Run(); err != nil {
			return fmt.Errorf("%v failed for %s: %w", argv[1:], path, err)
		}
	}
	for _, argv := range optional {
		// Removing an ACE that isn't present is a harmless no-op.
		_ = exec.Command(argv[0], argv[1:]...).Run()
	}
	return nil
}

// PermsOK - see windowsPermsOK for why any existing file passes.
func PermsOK(path string, _ os.FileMode) bool { return windowsPermsOK(path) }
