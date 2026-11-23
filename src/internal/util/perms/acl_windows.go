//go:build windows

package perms

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ownerOnlyACL reads path's owner and DACL and applies aclOwnerOnly.
func ownerOnlyACL(path string) (bool, string) {
	if _, err := os.Stat(path); err != nil {
		return false, err.Error()
	}
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return false, "cannot read the current user: " + err.Error()
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, "cannot read its ACL: " + err.Error()
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return false, "cannot read its owner"
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return false, "cannot read its DACL: " + err.Error()
	}
	var entries []aclEntry
	if dacl != nil {
		for i := uint32(0); i < uint32(dacl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(dacl, i, &ace); err != nil {
				return false, "cannot read an ACE: " + err.Error()
			}
			e := aclEntry{
				Allow:       ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE,
				InheritOnly: ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0,
			}
			// Allowed and denied ACEs share this layout. Any other type that
			// grants access (compound, object, callback) is one this check
			// cannot read the trustee of, so the file is not owner-only by it.
			switch ace.Header.AceType {
			case windows.ACCESS_ALLOWED_ACE_TYPE, windows.ACCESS_DENIED_ACE_TYPE:
				e.SID = (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String()
			case aceAllowedCompound, aceAllowedObject, aceAllowedCallback, aceAllowedCallbackObject:
				if !e.InheritOnly {
					return false, fmt.Sprintf("it has an access-granting ACE of type %d that authama cannot check", ace.Header.AceType)
				}
				continue
			default:
				continue // denies and audit entries grant nothing
			}
			entries = append(entries, e)
		}
	}
	return aclOwnerOnly(tu.User.Sid.String(), owner.String(), dacl == nil, entries)
}

// The access-granting ACE types besides ACCESS_ALLOWED_ACE_TYPE (winnt.h).
const (
	aceAllowedCompound       = 4
	aceAllowedObject         = 5
	aceAllowedCallback       = 9
	aceAllowedCallbackObject = 11
)
