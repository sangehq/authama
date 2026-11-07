// Package paths resolves the per-user directories and the on-disk layout.
//
// Two roots, on every OS (Windows included - %USERPROFILE% is its ~):
//
//	config home  $AUTHAMA_CONFIG_DIR, else $XDG_CONFIG_HOME/authama, else
//	             ~/.config/authama. What the user owns and edits: the
//	             manifest, inventory, providers, tokens config, .env,
//	             authama.json, the age identity.
//	state home   $AUTHAMA_STATE_DIR, else $XDG_STATE_HOME/authama, else
//	             ~/.local/state/authama. What the tool writes for itself: the
//	             history database and audit log (log/), snapshots, bundles
//	             (dist/), the lock and caches (.state/).
//
// When the config home is set explicitly ($AUTHAMA_CONFIG_DIR or a legacy
// spelling) and the state home is not, the state lives inside the config home,
// as it did before 0.1.1, so a pinned home stays self-contained. A --dev-root
// sandbox is likewise one tree.
//
// The legacy variables $SSH_MANAGER_HOME and $SSH_MANAGER_CONFIG_DIR are still
// honored (see envnames); the folders earlier releases used (~/.config/
// ssh-manager, %APPDATA%\ssh-manager, ~/.config/sshmgr, ~/.sshmgr) are found by
// LegacyHomes for the one-time migration.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/util/envnames"
)

// Folder is the directory name under the config and state bases.
const Folder = "authama"

// LegacyFolder is the config folder name releases before 0.1.1 used.
const LegacyFolder = "ssh-manager"

// MigratedMarker is written into a legacy home once its contents have been
// copied to the current one. The legacy folder is kept, but a marked folder is
// never offered for migration again and doctor does not call it stranded.
const MigratedMarker = "MIGRATED-TO-AUTHAMA.txt"

// Getenv is an env lookup, injectable for tests. nil means os.Getenv.
type Getenv func(string) string

func resolve(get Getenv) Getenv {
	if get == nil {
		return os.Getenv
	}
	return get
}

func home(get Getenv) string {
	if platform.IsWindows() {
		if v := get("USERPROFILE"); v != "" {
			return v
		}
	} else if v := get("HOME"); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	return h
}

func expandUser(p string, get Getenv) string {
	if p == "~" {
		return home(get)
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(home(get), p[2:])
	}
	return p
}

// ConfigDir resolves the per-user config home. cwd is used to absolutize a
// relative override (pass "" to use the process cwd).
func ConfigDir(get Getenv, cwd string) string {
	dir, _ := configDir(resolve(get), cwd)
	return dir
}

// configDir also reports whether the home was set explicitly.
func configDir(get Getenv, cwd string) (string, bool) {
	if override := envnames.Get(envnames.Getenv(get), envnames.ConfigDir); override != "" {
		return absolutize(expandUser(override, get), cwd), true
	}
	return filepath.Join(xdgBase(get, "XDG_CONFIG_HOME", ".config"), Folder), false
}

// xdgBase is $<variable> when it is set to an absolute path (the XDG spec
// says a relative one is to be ignored), else ~/<fallback>. It is the same on
// every OS, Windows included.
func xdgBase(get Getenv, variable, fallback string) string {
	if v := get(variable); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home(get), fallback)
}

func absolutize(p, cwd string) string {
	if filepath.IsAbs(p) {
		return p
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return filepath.Join(cwd, p)
}

// StateRoot resolves the state home for a config home: $AUTHAMA_STATE_DIR,
// else the config home itself when that was set explicitly, else
// $XDG_STATE_HOME/authama or ~/.local/state/authama.
func StateRoot(get Getenv, cwd string) string {
	get = resolve(get)
	if v := get(envnames.StateDir); v != "" {
		return absolutize(expandUser(v, get), cwd)
	}
	if dir, explicit := configDir(get, cwd); explicit {
		return dir
	}
	return filepath.Join(xdgBase(get, "XDG_STATE_HOME", filepath.Join(".local", "state")), Folder)
}

// Paths is the resolved on-disk layout for one invocation.
type Paths struct {
	SSHDir    string
	ConfigDir string
	// StateRoot holds what the tool writes for itself (log/, snapshots/,
	// dist/, .state/). Empty means ConfigDir: a Paths built by hand, as the
	// preview copy and the tests do, keeps the single-tree layout.
	StateRoot string
	// DevRoot is the sandbox root when this layout came from --dev-root, and
	// empty otherwise. Its presence is what every "am I sandboxed" check reads,
	// so there is one answer rather than a comparison rebuilt per call site.
	DevRoot string
}

// Resolve builds the Paths bundle. sshDir "" defaults to ~/.ssh.
func Resolve(get Getenv, cwd, sshDir string) Paths {
	get = resolve(get)
	if sshDir == "" {
		sshDir = filepath.Join(home(get), ".ssh")
	}
	return Paths{SSHDir: sshDir, ConfigDir: ConfigDir(get, cwd), StateRoot: StateRoot(get, cwd)}
}

// State is the effective state home.
func (p Paths) State() string {
	if p.StateRoot == "" {
		return p.ConfigDir
	}
	return p.StateRoot
}

// SeparateState reports whether p's state home is not its config home. A
// function, not a method: Paths stays a list of locations.
func SeparateState(p Paths) bool {
	return filepath.Clean(p.State()) != filepath.Clean(p.ConfigDir)
}

// SettingsFile is authama.json, the user's settings file.
func (p Paths) SettingsFile() string { return filepath.Join(p.ConfigDir, "authama.json") }

func (p Paths) Home() string         { return p.ConfigDir }
func (p Paths) Manifest() string     { return filepath.Join(p.ConfigDir, "manifest.json") }
func (p Paths) Inventory() string    { return filepath.Join(p.ConfigDir, "inventory.json") }
func (p Paths) Providers() string    { return filepath.Join(p.ConfigDir, "providers.json") }
func (p Paths) EnvFile() string      { return filepath.Join(p.ConfigDir, ".env") }
func (p Paths) AgeIdentity() string  { return filepath.Join(p.ConfigDir, "age-identity.txt") }
func (p Paths) LogDir() string       { return filepath.Join(p.State(), "log") }
func (p Paths) AuditLog() string     { return filepath.Join(p.LogDir(), "audit.log") }
func (p Paths) HistoryDB() string    { return filepath.Join(p.LogDir(), "history.db") }
func (p Paths) SnapshotsDir() string { return filepath.Join(p.State(), "snapshots") }
func (p Paths) DistDir() string      { return filepath.Join(p.State(), "dist") }

// ExportSelectionsDir holds p's saved export selections, one <name>.json
// each. A folder of their own: exports/ also holds the file `config export
// --flat` writes. A function, not a method: Paths stays
// a list of locations under its method ceiling.
func ExportSelectionsDir(p Paths) string { return filepath.Join(p.State(), "exports", "selections") }
func (p Paths) StateDir() string         { return filepath.Join(p.State(), ".state") }

// StateSubdirs are the state home's entries, by name. A config home from
// before 0.1.1 held them, and the migration moves them to a separate state home.
var StateSubdirs = []string{"log", "snapshots", "dist", ".state"}

func (p Paths) LockFile() string    { return filepath.Join(p.StateDir(), ".lock") }
func (p Paths) ExpiryCache() string { return filepath.Join(p.StateDir(), "expiry-cache.json") }
func (p Paths) NotifyCache() string { return filepath.Join(p.StateDir(), "notify-cache.json") }

// Tokens is the token config: which env vars build which registry credential.
// It holds variable names, never secrets.
func (p Paths) Tokens() string { return filepath.Join(p.ConfigDir, "tokens.json") }

// TokenHome is the directory the token targets (~/.npmrc, ~/.docker, ...)
// resolve under: the real home, or in dev mode the sandbox's "home" subtree,
// so a sandboxed token write can never reach a real credential file.
func (p Paths) TokenHome(get Getenv) string {
	if p.IsDev() {
		return filepath.Join(p.DevRoot, DevHomeDir)
	}
	return home(resolve(get))
}

// Contains reports whether path is at or below the sandbox root, after
// resolving symlinks in the part of path that exists. Outside dev mode every
// path qualifies.
func (p Paths) Contains(path string) bool {
	if !p.IsDev() {
		return true
	}
	return within(resolveExisting(p.DevRoot), resolveExisting(path))
}

// resolveExisting resolves symlinks in the longest existing prefix of path
// and re-attaches the rest.
func resolveExisting(path string) string {
	path = filepath.Clean(path)
	rest := ""
	for cur := path; ; {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return path
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// LegacyHomes are the homes earlier releases used, newest first: the
// "ssh-manager" sibling of the config home (0.1.0 and the ssh-manager
// releases), then on Windows %APPDATA%\ssh-manager (their Windows default),
// then the "sshmgr" sibling and %APPDATA%\sshmgr (before that rename), and the
// original dot-home ~/.sshmgr.
//
// A sandbox has none outside its root: ~/.sshmgr is the real user's, and
// dev-mode `migrate` used to move it into the sandbox.
func (p Paths) LegacyHomes() []string {
	parent := filepath.Dir(p.ConfigDir)
	if p.IsDev() {
		return []string{filepath.Join(parent, LegacyFolder), filepath.Join(parent, "sshmgr")}
	}
	out := []string{filepath.Join(parent, LegacyFolder)}
	appdata := ""
	if platform.IsWindows() {
		if appdata = os.Getenv("APPDATA"); appdata == "" {
			if h, err := os.UserHomeDir(); err == nil {
				appdata = filepath.Join(h, "AppData", "Roaming")
			}
		}
	}
	if appdata != "" {
		out = append(out, filepath.Join(appdata, LegacyFolder))
	}
	out = append(out, filepath.Join(parent, "sshmgr"))
	if appdata != "" {
		out = append(out, filepath.Join(appdata, "sshmgr"))
	}
	home, _ := os.UserHomeDir()
	return append(out, filepath.Join(home, ".sshmgr"))
}

// FirstLegacyHome returns the first legacy home still worth migrating (a real
// directory, not a symlink, not the current home, not already marked as
// migrated), or "".
func (p Paths) FirstLegacyHome() string {
	for _, cand := range p.LegacyHomes() {
		if filepath.Clean(cand) == filepath.Clean(p.ConfigDir) {
			continue
		}
		fi, err := os.Lstat(cand)
		if err != nil || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(cand, MigratedMarker)); err == nil {
			continue
		}
		return cand
	}
	return ""
}

// --- dev mode ---------------------------------------------------------------

// DevRootEnv is the environment variable form of --dev-root. Its pre-0.1.1
// spelling, SSHMGR_DEV_ROOT, is still read (envnames.Legacy).
const DevRootEnv = envnames.DevRoot

// DevSSHDir and DevConfigDir are the fixed subdirectories a dev root is split
// into.
const (
	DevSSHDir    = "ssh"
	DevConfigDir = "config"
	// DevHomeDir stands in for $HOME for the token targets.
	DevHomeDir = "home"
)

// ResolveDev builds the layout for a sandboxed run: one root, holding both the
// sandbox's ~/.ssh and its config home.
//
// It is deliberately ONE root rather than two independent overrides. The config
// home already had an override ($SSH_MANAGER_HOME) and ~/.ssh had none, so the
// only way to test against a scratch tree was to move $HOME - which also moves
// the ssh-agent socket, the launchd session and everything else that reads it.
// Two knobs would allow the worse failure: a run with its config redirected and
// its keys still going to the real ~/.ssh, which looks sandboxed right up until
// it overwrites a key you use.
//
// The root is created on demand by the caller; this only computes and checks it.
func ResolveDev(get Getenv, cwd, devRoot string) (Paths, error) {
	get = resolve(get)
	root := expandUser(devRoot, get)
	if !filepath.IsAbs(root) {
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		// On Windows `\scratch` is rooted but not absolute - Go wants a volume
		// too - and it means "that location on the current drive". Joining it to
		// the working directory instead silently turns it into a relative path,
		// so someone who typed `\` for the drive root would sandbox their
		// current directory and be told nothing.
		if vol := filepath.VolumeName(cwd); vol != "" && isRooted(root) {
			root = vol + root
		} else {
			root = filepath.Join(cwd, root)
		}
	}
	root = filepath.Clean(root)

	real := Resolve(get, cwd, "")
	if err := checkDevRoot(root, real); err != nil {
		return Paths{}, err
	}
	return Paths{
		SSHDir:    filepath.Join(root, DevSSHDir),
		ConfigDir: filepath.Join(root, DevConfigDir),
		StateRoot: filepath.Join(root, DevConfigDir),
		DevRoot:   root,
	}, nil
}

// checkDevRoot refuses a sandbox that is not one.
//
// Both directions matter. A root inside the real ~/.ssh would have the tool
// write keys into the tree it is supposed to be avoiding; a root that CONTAINS
// the real ~/.ssh (the home directory, or /) means a later snapshot, clean or
// restore walks the real tree while believing it is scratch.
func checkDevRoot(root string, real Paths) error {
	// A filesystem root, checked after the path has been absolutized rather than
	// by comparing the input to a separator. On Windows the input never looks
	// like the root it resolves to: `\` has no volume, so the literal
	// comparison matched nothing and the whole guard was dead there.
	// Dir(root) == root is true at "/" and at "C:\" alike.
	if root == "" || filepath.Dir(root) == root {
		return fmt.Errorf("--dev-root %q is not a usable sandbox: it is a filesystem root", root)
	}
	for _, danger := range []struct{ path, what string }{
		{real.SSHDir, "your real ~/.ssh"},
		{real.ConfigDir, "your real config home"},
		{real.StateRoot, "your real state home"},
	} {
		if danger.path == "" {
			continue
		}
		if within(danger.path, root) {
			return fmt.Errorf("--dev-root %s is inside %s (%s); "+
				"a sandbox there would write to the tree it exists to avoid",
				root, danger.what, danger.path)
		}
		if within(root, danger.path) {
			return fmt.Errorf("--dev-root %s contains %s (%s); "+
				"a snapshot or restore under it would walk the real tree",
				root, danger.what, danger.path)
		}
	}
	return nil
}

// within reports whether dest is at or below root.
//
// This is a check on two paths the tool computed itself - a sandbox root against
// the real home - so comparing the spellings is the whole question. It is not
// the right tool for a path that came out of an archive: see the extractors in
// services/snapshots and services/bundler, which confine writes with os.Root
// because a textually-inside path can still resolve outside through a symlink.
func within(root, dest string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(dest))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

// isRooted reports whether p begins at a path separator without naming a
// volume - the Windows "\somewhere" form.
func isRooted(p string) bool {
	return p != "" && (p[0] == '\\' || p[0] == '/') && filepath.VolumeName(p) == ""
}

// IsDev reports whether this layout is a sandbox.
func (p Paths) IsDev() bool { return p.DevRoot != "" }

// RefuseOutsideSandbox refuses, in dev mode, an action that takes effect on a
// server or account. Dev mode sandboxes files; it cannot sandbox a key
// installed on a real host or removed from one. It lives on Paths, and the
// services that reach targets (deploy, rotate, rollback, revoke) call it
// themselves, so no caller - a verb, the TUI, a future one - can skip it.
func (p Paths) RefuseOutsideSandbox(action string) error {
	if !p.IsDev() {
		return nil
	}
	return fmt.Errorf("%s is refused in dev mode: it changes keys on real servers or accounts, "+
		"which the sandbox under %s cannot contain", action, p.DevRoot)
}
