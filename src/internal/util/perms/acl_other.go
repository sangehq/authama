//go:build !windows

package perms

import "os"

// ownerOnlyACL is the Windows ACL check off Windows, where there is no ACL
// to read: the file only has to exist. The rules themselves (aclOwnerOnly)
// are tested on every host; the read is tested on the Windows CI leg.
func ownerOnlyACL(path string) (bool, string) {
	if _, err := os.Stat(path); err != nil {
		return false, err.Error()
	}
	return true, ""
}
