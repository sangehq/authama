package owner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ServiceName is the service the sealing key is kept under in the macOS
// Keychain and the Secret Service, with the config home as the account.
const ServiceName = "authama-owner-key"

// dpapiPath is where Windows keeps the key, DPAPI-encrypted for the account.
func dpapiPath(p paths.Paths) string { return filepath.Join(p.StateDir(), "owner.key.dpapi") }

// orphanStores are the OS credential stores a sealing key could be left in
// once the files that name it are gone. Asking one can mean a keychain
// prompt, so it is asked only by SealRemoved, never per request. A test
// replaces it with a store of its own, so no test touches the real keychain.
var orphanStores = func(p paths.Paths) []Store {
	if p.IsDev() || strings.EqualFold(envnames.Get(nil, StoreEnv), StoreFile) {
		return nil
	}
	if s := osStore(p); s != nil {
		return []Store{s}
	}
	return nil
}

// keyLeftInOSStore names the OS store still holding a sealing key for this
// home, or "". A store that cannot be read (a locked keychain) is not taken
// to hold one.
func keyLeftInOSStore(p paths.Paths) string {
	for _, st := range orphanStores(p) {
		if _, err := st.Load(); err == nil {
			return st.Name()
		}
	}
	return ""
}

// SealRemovedError is a home whose seal was removed while what bound it is
// still there: a forget that stopped part-way, or somebody deleting the files
// to switch the seal and the PIN off (audit A-SEC-08). It says what to do.
type SealRemovedError struct {
	p paths.Paths
	// Detail is what is gone and what is left.
	Detail string
	// Bound is a home whose owner.json is still there (only seal.json is gone).
	Bound bool
}

func (e *SealRemovedError) Error() string {
	return "the seal was removed: " + e.Detail + ".\n" + e.Advice()
}

// Advice is what the owner can do: put the files back, bind afresh on
// purpose, or clear what is left. Every command is spelled out.
func (e *SealRemovedError) Advice() string {
	p := e.p
	if e.Bound {
		return "What to do:\n" +
			"  - If you did not remove it, put " + SealPath(p) + " back from a backup of this home, then run `authama owner verify`.\n" +
			"  - If you know the files are as you left them, `authama owner reseal` seals them as they are now (it asks for your PIN).\n" +
			"  - To unbind the home instead: `authama owner forget`."
	}
	// The account is the config home, as store_account.go names it.
	acct := shellQuote(p.ConfigDir)
	return "What to do:\n" +
		"  - If you did not remove them, put them back from a backup of this home (Time Machine, restic, a copy), then run `authama owner verify`:\n" +
		"      " + ConfigPath(p) + "\n" +
		"      " + SealPath(p) + "\n" +
		"      " + attemptsPath(p) + "\n" +
		"    authama's own snapshots and bundles do not carry these files.\n" +
		"  - If you meant to start again, bind the home afresh: `authama owner init`, then `authama owner pin set`. init replaces the key left behind.\n" +
		"  - If you meant to unbind it, finish with `authama owner forget`, which removes the key left behind (it asks you to type \"forget\").\n" +
		"    The key by hand, wherever it is kept:\n" +
		"      macOS Keychain:        security delete-generic-password -a " + acct + " -s " + ServiceName + "\n" +
		"      Linux Secret Service:  secret-tool clear service " + ServiceName + " account " + acct + "\n" +
		"      Windows (DPAPI; authama keeps nothing in Credential Manager):  Remove-Item " + shellQuote(dpapiPath(p)) + "\n" +
		"      the owner-only file:   rm " + shellQuote(fileStore{p}.path())
}

// shellQuote quotes s for a POSIX shell (and PowerShell, which reads '...'
// the same way unless s holds a quote).
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// SealRemoved reports a home whose seal was removed, as a *SealRemovedError,
// or nil. Unlike Orphaned it also asks the OS credential store, so it can see
// a key left there when owner.json, seal.json and pin-attempts.json were all
// deleted (audit A-SEC-08). That can prompt a keychain, so it runs where a
// person asked for a check: `owner verify`, `doctor`, and `serve` starting.
// An owner.json that cannot be read is Verify's to report, not this.
func SealRemoved(p paths.Paths) error {
	c, err := Load(p)
	if err != nil {
		return nil
	}
	if c != nil {
		if _, err := os.Lstat(SealPath(p)); errors.Is(err, os.ErrNotExist) {
			return &SealRemovedError{p: p, Bound: true, Detail: "owner.json binds this home, but " + SealPath(p) + " is gone"}
		}
		return nil
	}
	if Orphaned(p) {
		return &SealRemovedError{p: p, Detail: ConfigPath(p) + " is gone, but the rest of the binding (the seal, the attempt count or the key file) is still here"}
	}
	if st := keyLeftInOSStore(p); st != "" {
		return &SealRemovedError{p: p, Detail: "owner.json, seal.json and pin-attempts.json are gone, but the sealing key for this home is still in " + st}
	}
	return nil
}
