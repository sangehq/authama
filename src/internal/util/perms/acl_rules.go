package perms

import "fmt"

// The Windows ACL check (P-04), as rules over SIDs so it is tested on every
// host. acl_windows.go reads a file's owner and DACL with the Windows API and
// hands them here; nothing in this file calls Windows.

// Well-known SIDs that may hold access to a private file. SYSTEM and the
// Administrators group can read any file on the machine regardless, so their
// ACEs grant nothing new; every other principal is someone else.
const (
	sidSystem         = "S-1-5-18"
	sidAdministrators = "S-1-5-32-544"
	sidOwnerRights    = "S-1-3-4"
)

// aclEntry is one ACE as the check needs it.
type aclEntry struct {
	Allow       bool   // an access-allowed ACE; deny ACEs only take access away
	InheritOnly bool   // applies to children only, not to this object
	SID         string // the principal, as S-1-...
}

// trustedSID reports whether sid may hold access to a file that is the user's
// alone: the user, SYSTEM, the Administrators group, or OWNER RIGHTS.
func trustedSID(sid, user string) bool {
	switch sid {
	case user, sidSystem, sidAdministrators, sidOwnerRights:
		return true
	}
	return false
}

// aclOwnerOnly is the Windows meaning of "mode 0600/0700": the file is owned
// by the user (or SYSTEM/Administrators), it has a DACL, and no ACE that
// applies to it grants anyone else access. nullDACL is a DACL that is not
// there at all, which Windows reads as full access for everyone.
func aclOwnerOnly(user, owner string, nullDACL bool, entries []aclEntry) (bool, string) {
	if user == "" {
		return false, "the current user's SID is unknown"
	}
	if nullDACL {
		return false, "it has no DACL, so everyone has full access"
	}
	if !trustedSID(owner, user) {
		return false, fmt.Sprintf("it is owned by %s, not by you", owner)
	}
	for _, e := range entries {
		if !e.Allow || e.InheritOnly {
			continue
		}
		if !trustedSID(e.SID, user) {
			return false, fmt.Sprintf("%s has access to it", sidName(e.SID))
		}
	}
	return true, ""
}

// sidName names the broad principals a finding most often reports.
func sidName(sid string) string {
	switch sid {
	case "S-1-1-0":
		return "Everyone (" + sid + ")"
	case "S-1-5-11":
		return "Authenticated Users (" + sid + ")"
	case "S-1-5-32-545":
		return "Users (" + sid + ")"
	}
	return sid
}
