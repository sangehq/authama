package perms

import (
	"os/user"
	"strings"
)

// The Windows ACL rules, kept out of the build-tagged file that runs them.
//
// perms_windows.go can only be compiled - and therefore only be tested - on
// Windows, which put the whole of this logic beyond reach of every developer
// machine and two of the three CI legs. What is actually worth checking is not
// that exec.Command works: it is which commands get built, in what order, and
// who they name. That is all decidable without Windows, so it lives here and
// perms_windows.go is left as the thin part that runs them.

// broadPrincipals are the over-broad ACEs stripped from keys, directories and
// configs so no other principal keeps access. Mirrors
// v1's windows layer (_BROAD_PRINCIPALS).
var broadPrincipals = []string{"Everyone", "Authenticated Users", "Users", `BUILTIN\Users`}

// aclOwner is the account an ACL is granted to: the current user's SID, as
// icacls spells one (*S-1-...), when Windows reports it; else %USERNAME% when
// set, else the current user's name. The SID comes first because a variable
// is anyone's to set, a name can be ambiguous between a local and a domain
// account, and a renamed account keeps its SID (P-04). getenv and current are
// injected so every fallback is testable.
func aclOwner(getenv func(string) string, current func() (*user.User, error)) string {
	cu, err := current()
	if err == nil && strings.HasPrefix(cu.Uid, "S-1-") {
		return "*" + cu.Uid
	}
	if u := getenv("USERNAME"); u != "" {
		return u
	}
	if err == nil {
		return cu.Username
	}
	return ""
}

// icaclsCommands is the argv of every icacls call that restricts path to owner,
// in the order they must run.
//
// Order is the whole point. Dropping inheritance first means the explicit grant
// that follows is the only thing left; granting first would leave a window where
// the file carries both the inherited ACEs and the new one. The broad-principal
// removals come last and are individually optional - removing an ACE that is not
// there is a no-op - so they are separated from the two that must succeed.
func icaclsCommands(path, owner string) (required [][]string, optional [][]string) {
	required = [][]string{
		{"icacls", path, "/inheritance:r"},
		{"icacls", path, "/grant:r", owner + ":F"},
	}
	for _, p := range broadPrincipals {
		optional = append(optional, []string{"icacls", path, "/remove:g", p})
	}
	return required, optional
}

// windowsPermsOK is the Windows stand-in for a mode check (P-04): on Windows
// it reads the file's owner and DACL and passes only when no one but you,
// SYSTEM and Administrators can reach it (aclOwnerOnly). Off Windows there is
// no ACL to read, so an existing file passes.
func windowsPermsOK(path string) bool {
	return WindowsPermsOK(path)
}

// WindowsPermsOK is windowsPermsOK, exported for the token checkup.
func WindowsPermsOK(path string) bool {
	ok, _ := ownerOnlyACL(path)
	return ok
}

// WindowsACLProblem says why path fails the Windows ACL check, or "" when it
// passes, for a finding to show.
func WindowsACLProblem(path string) string {
	_, why := ownerOnlyACL(path)
	return why
}
