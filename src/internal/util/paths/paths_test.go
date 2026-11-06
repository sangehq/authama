package paths

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/envnames"
)

func env(m map[string]string) Getenv {
	return func(k string) string { return m[k] }
}

func TestConfigDirDefaults(t *testing.T) {
	// One layout on every OS, Windows included: ~/.config/authama, or
	// $XDG_CONFIG_HOME/authama when that is an absolute path.
	h := absPath("tmp", "h")
	base := map[string]string{"HOME": h, "USERPROFILE": h, "APPDATA": absPath("AppData")}
	if got, want := ConfigDir(env(base), absPath("cwd")), filepath.Join(h, ".config", "authama"); got != want {
		t.Fatalf("default = %q, want %q", got, want)
	}
	xdg := absPath("tmp", "xdg")
	withXDG := map[string]string{"HOME": h, "USERPROFILE": h, "XDG_CONFIG_HOME": xdg}
	if got, want := ConfigDir(env(withXDG), absPath("cwd")), filepath.Join(xdg, "authama"); got != want {
		t.Fatalf("XDG = %q, want %q", got, want)
	}
	// A relative XDG_CONFIG_HOME is ignored, as the spec says.
	rel := map[string]string{"HOME": h, "USERPROFILE": h, "XDG_CONFIG_HOME": "rel"}
	if got, want := ConfigDir(env(rel), absPath("cwd")), filepath.Join(h, ".config", "authama"); got != want {
		t.Fatalf("relative XDG = %q, want %q", got, want)
	}
}

// The state home is apart from the config home by default, and inside it when
// the config home is pinned, so a pinned home stays one self-contained tree.
func TestStateRootFollowsTheDocumentedOrder(t *testing.T) {
	h := absPath("tmp", "h")
	cwd := absPath("cwd")
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"default", map[string]string{"HOME": h, "USERPROFILE": h}, filepath.Join(h, ".local", "state", "authama")},
		{"xdg", map[string]string{"HOME": h, "USERPROFILE": h, "XDG_STATE_HOME": absPath("st")}, filepath.Join(absPath("st"), "authama")},
		{"explicit", map[string]string{"HOME": h, "USERPROFILE": h, "AUTHAMA_STATE_DIR": absPath("s2")}, absPath("s2")},
		{"pinned config home", map[string]string{"HOME": h, "USERPROFILE": h, "AUTHAMA_CONFIG_DIR": absPath("c")}, absPath("c")},
		{"legacy pinned home", map[string]string{"HOME": h, "USERPROFILE": h, "SSH_MANAGER_HOME": absPath("c")}, absPath("c")},
		{"explicit wins over pinned", map[string]string{"HOME": h, "USERPROFILE": h, "AUTHAMA_CONFIG_DIR": absPath("c"), "AUTHAMA_STATE_DIR": absPath("s")}, absPath("s")},
	}
	for _, c := range cases {
		if got := StateRoot(env(c.env), cwd); got != c.want {
			t.Errorf("%s: StateRoot = %q, want %q", c.name, got, c.want)
		}
	}
	p := Resolve(env(map[string]string{"HOME": h, "USERPROFILE": h}), cwd, "")
	if !SeparateState(p) || p.LogDir() != filepath.Join(h, ".local", "state", "authama", "log") {
		t.Errorf("default layout: separate=%v log=%q", SeparateState(p), p.LogDir())
	}
	if p.Manifest() != filepath.Join(h, ".config", "authama", "manifest.json") || p.SettingsFile() != filepath.Join(h, ".config", "authama", "authama.json") {
		t.Errorf("config files: %q %q", p.Manifest(), p.SettingsFile())
	}
}

// absPath builds an OS-absolute path (drive-letter rooted on Windows).
func absPath(parts ...string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(append([]string{`C:\`}, parts...)...)
	}
	return filepath.Join(append([]string{"/"}, parts...)...)
}

func TestConfigDirOverride(t *testing.T) {
	abs := absPath("abs", "home")
	cwd := absPath("cwd")
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"AUTHAMA_CONFIG_DIR": abs}, abs},
		{map[string]string{"SSH_MANAGER_HOME": abs}, abs},
		{map[string]string{"SSH_MANAGER_CONFIG_DIR": abs}, abs},                   // alias
		{map[string]string{"SSH_MANAGER_HOME": "rel"}, filepath.Join(cwd, "rel")}, // relative absolutized
	}
	for _, c := range cases {
		if got := ConfigDir(env(c.env), cwd); got != c.want {
			t.Errorf("ConfigDir(%v) = %q, want %q", c.env, got, c.want)
		}
	}
	// An empty override falls through to the OS default (not treated as set).
	got := ConfigDir(env(map[string]string{"SSH_MANAGER_HOME": "", "HOME": "/tmp/h", "APPDATA": `C:\A`}), cwd)
	if filepath.Base(got) != "authama" {
		t.Errorf("empty override should fall through to default, got %q", got)
	}
}

func TestPathsLayout(t *testing.T) {
	h := absPath("home", "x")
	ssh := absPath("ssh")
	p := Resolve(env(map[string]string{"SSH_MANAGER_HOME": h}), absPath("cwd"), ssh)
	checks := map[string]string{
		"home":        p.Home(),
		"manifest":    p.Manifest(),
		"env":         p.EnvFile(),
		"auditLog":    p.AuditLog(),
		"lockFile":    p.LockFile(),
		"distDir":     p.DistDir(),
		"expiryCache": p.ExpiryCache(),
	}
	want := map[string]string{
		"home":        h,
		"manifest":    filepath.Join(h, "manifest.json"),
		"env":         filepath.Join(h, ".env"),
		"auditLog":    filepath.Join(h, "log", "audit.log"),
		"lockFile":    filepath.Join(h, ".state", ".lock"),
		"distDir":     filepath.Join(h, "dist"),
		"expiryCache": filepath.Join(h, ".state", "expiry-cache.json"),
	}
	for k := range want {
		if checks[k] != want[k] {
			t.Errorf("%s = %q, want %q", k, checks[k], want[k])
		}
	}
	if p.SSHDir != ssh {
		t.Errorf("SSHDir = %q, want %q", p.SSHDir, ssh)
	}
}

// isolateHome points the real home lookup at a clean directory, so ~/.sshmgr on
// the machine running the tests cannot make FirstLegacyHome return something the
// test did not create.
func isolateHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

// FirstLegacyHome decides what `migrate` moves - it renames the directory it
// names, and with --force replaces the current home with it. Each exclusion
// exists to stop that happening to something it should not: a regular file left
// at the legacy path, a symlink whose target lives anywhere at all, and the
// standard home itself when the two paths coincide.
func TestFirstLegacyHomeSkipsWhatItMustNotMigrate(t *testing.T) {
	isolateHome(t)
	base := t.TempDir()
	p := Paths{ConfigDir: filepath.Join(base, "ssh-manager")}
	sibling := filepath.Join(base, "sshmgr")

	if got := p.FirstLegacyHome(); got != "" {
		t.Errorf("nothing exists yet, got %q", got)
	}

	// A regular file at the legacy path is not a home to migrate.
	if err := os.WriteFile(sibling, []byte("not a directory\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := p.FirstLegacyHome(); got != "" {
		t.Errorf("a regular file was offered for migration: %q", got)
	}
	if err := os.Remove(sibling); err != nil {
		t.Fatal(err)
	}

	// A symlink is refused whatever it points at: migrate renames what it is
	// given, which for a link means moving the link and leaving the real
	// directory stranded, or - with --force - replacing the home with one.
	if runtime.GOOS != "windows" {
		target := filepath.Join(base, "elsewhere")
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, sibling); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if got := p.FirstLegacyHome(); got != "" {
			t.Errorf("a symlinked legacy home was offered for migration: %q", got)
		}
		if err := os.Remove(sibling); err != nil {
			t.Fatal(err)
		}
	}

	// A real directory is what it is looking for.
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := p.FirstLegacyHome(); got != sibling {
		t.Errorf("FirstLegacyHome = %q, want %q", got, sibling)
	}

	// When the standard home *is* the sibling, there is nothing to migrate - and
	// migrating a directory onto itself would destroy it.
	self := Paths{ConfigDir: sibling}
	if got := self.FirstLegacyHome(); got == sibling {
		t.Error("the standard home was offered as its own legacy home")
	}
}

// A home override is something a user types, and both `~/sshmgr-home` and
// `~\sshmgr-home` are things they type. Honouring only the forward-slash form
// leaves a literal tilde in the path, so every file lands in a directory called
// "~" beside wherever the process happened to be started.
func TestATildeOverrideExpandsWithEitherSeparator(t *testing.T) {
	h := absPath("users", "me")
	e := env(map[string]string{"HOME": h, "USERPROFILE": h})
	cwd := absPath("cwd")

	for _, in := range []string{"~/sshmgr-home", `~\sshmgr-home`} {
		want := filepath.Join(h, "sshmgr-home")
		got := ConfigDir(env(map[string]string{
			"HOME": h, "USERPROFILE": h, "SSH_MANAGER_HOME": in,
		}), cwd)
		if got != want {
			t.Errorf("ConfigDir(%q) = %q, want %q", in, got, want)
		}
	}
	// A bare "~" is the home itself, not a directory named "~" inside it.
	if got := expandUser("~", e); got != h {
		t.Errorf(`expandUser("~") = %q, want %q`, got, h)
	}
	// Another user's home is not ours to expand.
	const other = "~someone/else"
	if got := expandUser(other, e); got != other {
		t.Errorf("expandUser(%q) = %q, want it left alone", other, got)
	}
}

// Resolve's default SSHDir is ~/.ssh, taken from the injected environment rather
// than the process's, so every command in one invocation agrees on which tree it
// is managing.
func TestResolveDefaultsSSHDirToTheHomeItWasGiven(t *testing.T) {
	h := absPath("users", "me")
	p := Resolve(env(map[string]string{
		"HOME": h, "USERPROFILE": h, "SSH_MANAGER_HOME": absPath("cfg"),
	}), absPath("cwd"), "")
	if want := filepath.Join(h, ".ssh"); p.SSHDir != want {
		t.Errorf("SSHDir = %q, want %q", p.SSHDir, want)
	}
}

// X1 - the public environment variables, as one table.
//
// These are the tool's configuration API: a user sets them in a shell profile or
// a CI job, and a rename is a silent break for everyone who did. The current
// names are AUTHAMA_*; the SSH_MANAGER_* / SSHMGR_* names of earlier releases
// are still read, and the current one wins when both are set.
func TestThePublicEnvironmentVariablesResolveAsDocumented(t *testing.T) {
	abs := absPath("elsewhere")
	cwd := absPath("cwd")
	for _, name := range []string{"AUTHAMA_CONFIG_DIR", "SSH_MANAGER_HOME", "SSH_MANAGER_CONFIG_DIR"} {
		if got := ConfigDir(env(map[string]string{name: abs}), cwd); got != abs {
			t.Errorf("%s = %q, want %q", name, got, abs)
		}
	}
	other := absPath("second")
	if got := ConfigDir(env(map[string]string{
		"AUTHAMA_CONFIG_DIR": abs, "SSH_MANAGER_HOME": other, "SSH_MANAGER_CONFIG_DIR": other,
	}), cwd); got != abs {
		t.Errorf("with all set the result was %q; AUTHAMA_CONFIG_DIR is the primary", got)
	}
	if got := ConfigDir(env(map[string]string{
		"SSH_MANAGER_HOME": abs, "SSH_MANAGER_CONFIG_DIR": other,
	}), cwd); got != abs {
		t.Errorf("legacy pair gave %q; SSH_MANAGER_HOME came first", got)
	}
}

// The names themselves, asserted against the source that reads them. A variable
// renamed in code and not here - or here and not in code - is a break for every
// user who set it, and nothing else in the suite would notice.
func TestTheEnvironmentVariableNamesAreTheDocumentedOnes(t *testing.T) {
	documented := map[string]string{
		"AUTHAMA_CONFIG_DIR":           "the config home; absolutised against cwd if relative",
		"AUTHAMA_STATE_DIR":            "the state home (history, logs, snapshots, bundles, lock)",
		"AUTHAMA_DEV_ROOT":             "the --dev-root sandbox",
		"AUTHAMA_NO_DOTENV":            "1 skips <config home>/.env",
		"AUTHAMA_AUTO_PIN":             "0 disables auto-pinning reachable hosts on reconcile",
		"AUTHAMA_VM_HYPERVISORS":       "the hypervisors vm list/sync/add use (comma-separated)",
		"AUTHAMA_VM_PRUNE_GONE":        "0 keeps a VM host whose VM is gone on vm sync",
		"AUTHAMA_VM_REPIN":             "1 lets vm sync replace a changed guest host key pin",
		"AUTHAMA_WEB_ADDR":             "the address serve listens on (web.addr)",
		"AUTHAMA_WEB_PORT":             "the port serve tries first (web.port)",
		"AUTHAMA_WEB_BASE_PATH":        "the path the web UI is served under (web.base_path)",
		"AUTHAMA_AGE_RECIPIENT":        "default age recipient for bundle",
		"AUTHAMA_AGE_IDENTITY_FILE":    "default age identity for restore",
		"AUTHAMA_SNAPSHOT_RETAIN":      "how many ~/.ssh snapshots to keep",
		"AUTHAMA_OLD_KEY_MAX_AGE_DAYS": "when an archived predecessor is called stale",
		"AUTHAMA_HISTORY":              "0 stops recording operations in log/history.db",
		"AUTHAMA_GIT_MODE":             "overrides defaults.git.mode for one command",
		"AUTHAMA_TOKENS_CONFIG":        "the token config file, ahead of ./authama-tokens.json and <home>/tokens.json",
		"AUTHAMA_TOKENS_CATALOG":       "a catalog.json replacing the built-in token services and providers",
		"AUTHAMA_DOCKER_CREDS_STORE":   "login or refuse, for registries in a Docker credential store",
		"AUTHAMA_NOTIFICATIONS":        "0 turns desktop notifications off",
		"AUTHAMA_OWNER_STORE":          "file keeps the owner's sealing key in a file, not the OS credential store",
		"AUTHAMA_ASKPASS_MODE":         "internal: marks the askpass re-execution",
		"AUTHAMA_ASKPASS_SECRET":       "internal: the passphrase handed to it",
		"AUTHAMA_WEB_MENU_EXPORT":      "web.menu_export: show Export in the web UI's menu",
		"AUTHAMA_HELLO_CHECK":          "internal: asks the Windows Hello script for availability only",
		"AUTHAMA_WEB_AUTO_RESTART":     "web.auto_restart: restart serve by itself when its binary is rebuilt",
		"AUTHAMA_SERVE_HANDOFF":        "internal: a restarting serve's token, port and instance, for the server replacing it",
		// Earlier names, still read.
		"SSH_MANAGER_HOME":                 "legacy AUTHAMA_CONFIG_DIR",
		"SSH_MANAGER_CONFIG_DIR":           "legacy alias of SSH_MANAGER_HOME",
		"SSHMGR_DEV_ROOT":                  "legacy AUTHAMA_DEV_ROOT",
		"SSH_MANAGER_AUTO_PIN":             "legacy",
		"SSH_MANAGER_AGE_RECIPIENT":        "legacy",
		"SSH_MANAGER_AGE_IDENTITY_FILE":    "legacy",
		"SSH_MANAGER_SNAPSHOT_RETAIN":      "legacy",
		"SSH_MANAGER_OLD_KEY_MAX_AGE_DAYS": "legacy",
		"SSH_MANAGER_HISTORY":              "legacy",
		"SSHMGR_GIT_MODE":                  "legacy",
		"SSH_MANAGER_TOKENS_CONFIG":        "legacy",
		"SSH_MANAGER_TOKENS_CATALOG":       "legacy",
		"SSHMGR_ASKPASS_MODE":              "legacy internal",
		"SSHMGR_ASKPASS_SECRET":            "legacy internal",
	}
	table := map[string]bool{}
	for _, n := range envnames.AllNames() {
		table[n] = true
		if _, ok := documented[n]; !ok {
			t.Errorf("%s is in envnames but not documented here", n)
		}
	}
	for n := range documented {
		if !table[n] {
			t.Errorf("%s is documented here but envnames does not read it", n)
		}
	}
	for name := range readEnvNamesFromSource(t) {
		if _, ok := documented[name]; !ok {
			t.Errorf("%s is spelled in the source but not documented here; "+
				"it is part of the tool's configuration API either way", name)
		}
	}
}

// readEnvNamesFromSource collects every AUTHAMA_* / SSH_MANAGER_* / SSHMGR_*
// literal in the non-test tree, so the table above is checked against the
// code rather than against itself.
func readEnvNamesFromSource(t *testing.T) map[string]bool {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\b(?:AUTHAMA|SSH_MANAGER|SSHMGR)_[A-Z][A-Z_]*[A-Z]\b`)
	found := map[string]bool{}
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				return rerr
			}
			for _, m := range re.FindAllString(string(b), -1) {
				found[m] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return found
}

// The 0.1.0 home (~/.config/ssh-manager) is the first legacy candidate, and a
// home already copied over carries a marker that takes it out of the running.
func TestTheSSHManagerHomeIsFoundUntilItIsMarked(t *testing.T) {
	isolateHome(t)
	base := t.TempDir()
	p := Paths{ConfigDir: filepath.Join(base, "authama")}
	old := filepath.Join(base, "ssh-manager")
	if err := os.MkdirAll(filepath.Join(base, "sshmgr"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := p.FirstLegacyHome(); got != old {
		t.Fatalf("FirstLegacyHome = %q, want the ssh-manager home %q first", got, old)
	}
	if err := os.WriteFile(filepath.Join(old, MigratedMarker), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := p.FirstLegacyHome(); got != filepath.Join(base, "sshmgr") {
		t.Errorf("a marked home is still offered: got %q", got)
	}
}

// A sandbox never reaches outside its root for a legacy home: dev-mode
// `migrate` used to find the real ~/.sshmgr and move it into the sandbox.
func TestDevModeFindsNoLegacyHomeOutsideItsRoot(t *testing.T) {
	isolateHome(t)
	home, _ := os.UserHomeDir()
	if err := os.MkdirAll(filepath.Join(home, ".sshmgr"), 0o700); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	p, err := ResolveDev(nil, "", root)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.FirstLegacyHome(); got != "" {
		t.Errorf("dev mode found a legacy home outside its root: %s", got)
	}
	for _, c := range p.LegacyHomes() {
		if rel, err := filepath.Rel(root, c); err != nil || strings.HasPrefix(rel, "..") {
			t.Errorf("legacy candidate %s is outside the sandbox %s", c, root)
		}
	}
	// Without dev mode the real one is still found, so migrate keeps working.
	if got := Resolve(nil, "", "").FirstLegacyHome(); got == "" {
		t.Error("outside dev mode ~/.sshmgr should still be found")
	}
}
