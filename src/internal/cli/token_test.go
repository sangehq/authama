package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
)

// The token group's CLI, tested the way auth-token-manager's own CLI tests
// drove it (tests/test_cli_actions.py, test_writers.py, test_templates.py,
// test_aliases.py), through the cobra tree.
//
// Every test runs with --dev-root: the target files resolve under the
// sandbox's home, the variables that relocate them (COMPOSER_HOME,
// DOCKER_CONFIG, NETRC, ...) are hidden, and a write outside the sandbox is
// refused. editHome also points HOME at a temp dir, so even a leak could not
// reach the developer's real ~/.npmrc, ~/.docker, ~/.netrc, ~/.git-credentials
// or auth.json.

type tokenBox struct {
	root string // the --dev-root
	home string // where the targets live inside it
	env  map[string]string
}

func tokenSandbox(t *testing.T) *tokenBox {
	t.Helper()
	editHome(t)
	b := &tokenBox{root: filepath.Join(t.TempDir(), "sandbox"), env: map[string]string{}}
	b.home = filepath.Join(b.root, "home")
	prevGet, prevLook, prevTTY, prevClip := tokenGetenv, tokenLookup, stdinIsTerminal, clipboardWrite
	t.Cleanup(func() {
		tokenGetenv, tokenLookup, stdinIsTerminal, clipboardWrite = prevGet, prevLook, prevTTY, prevClip
	})
	tokenGetenv = func(k string) string { return b.env[k] }
	tokenLookup = func(k string) (string, bool) { v, ok := b.env[k]; return v, ok }
	stdinIsTerminal = func() bool { return false }
	clipboardWrite = func(string) error { t.Error("clipboard reached without a stub"); return nil }
	return b
}

// run returns stdout and stderr together, as a user reads them; stdout is
// what a pipe gets, and what JSON output is parsed from.
func (b *tokenBox) run(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	out, errOut, err := b.streams(t, stdin, args...)
	return errOut + out, err
}

func (b *tokenBox) streams(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"--dev-root", b.root, tokenGroup}, args...))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func (b *tokenBox) ok(t *testing.T, stdin string, args ...string) string {
	t.Helper()
	out, err := b.run(t, stdin, args...)
	if err != nil {
		t.Fatalf("token %v: %v\n%s", args, err, out)
	}
	return out
}

func (b *tokenBox) read(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(b.home, rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

func (b *tokenBox) inventory(t *testing.T, args ...string) []tokens.Installed {
	t.Helper()
	out, errOut, err := b.streams(t, "", append([]string{"inventory", "--json"}, args...)...)
	if err != nil {
		t.Fatalf("inventory --json: %v\n%s", err, errOut)
	}
	var items []tokens.Installed
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("inventory --json: %v\n%s", err, out)
	}
	return items
}

func (b *tokenBox) file(t *testing.T, target string) string {
	t.Helper()
	for _, it := range b.inventory(t) {
		if it.Target == target {
			return it.File
		}
	}
	t.Fatalf("no %s entry installed", target)
	return ""
}

func (b *tokenBox) config(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(b.root, "c.json")
	if err := os.MkdirAll(b.root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (b *tokenBox) project(t *testing.T) string {
	t.Helper()
	p := filepath.Join(b.root, "proj")
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- store / inventory / forget / clip (test_cli_actions.py) -------------

func TestTokenStoreGitFromStdin(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_secret\n", "store", "--target", "git", "--server", "github.com", "--username", "octocat")
	if got := b.read(t, ".git-credentials"); !strings.Contains(got, "octocat:ghp_secret@github.com") {
		t.Errorf(".git-credentials = %q", got)
	}
}

func TestTokenStoreComposerViaService(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_composer\n", "store", "--service", "github", "--server", "github.com")
	data, err := os.ReadFile(b.file(t, tokens.Composer))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["github-oauth"]["github.com"] != "ghp_composer" {
		t.Errorf("auth.json = %s", data)
	}
	if !strings.HasPrefix(b.file(t, tokens.Composer), b.home) {
		t.Errorf("composer's file %s is outside the sandbox home %s", b.file(t, tokens.Composer), b.home)
	}
}

func TestTokenStoreNPMWithScope(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "npmtok\n", "store", "--target", "npm", "--server", "npm.pkg.github.com", "--scope", "@org")
	got := b.read(t, ".npmrc")
	if !strings.Contains(got, "@org:registry") || !strings.Contains(got, "npmtok") {
		t.Errorf(".npmrc = %q", got)
	}
}

// ATM exited 2 for these; every failure exits 1 here (Execute), so the test
// is that each is refused and wrote nothing.
func TestTokenStoreRefusesIncompleteRequests(t *testing.T) {
	b := tokenSandbox(t)
	for _, tc := range []struct {
		stdin string
		args  []string
		want  string
	}{
		{"x\n", []string{"--target", "git", "--username", "u"}, "--server is required"},
		{"x\n", []string{"--target", "composer", "--server", "x"}, "needs --service"},
		{"x\n", []string{"--service", "nope", "--server", "x"}, `unknown service "nope"`},
		{"pw\n", []string{"--target", "docker", "--server", "ghcr.io"}, "--username is required"},
		{"\n", []string{"--target", "npm", "--server", "registry.npmjs.org"}, "token is empty"},
		{"x\n", []string{"--target", "npm", "--server", "r", "--creds-store-mode", "maybe"}, "--creds-store-mode must be"},
		{"x\n", []string{"--target", "nope", "--server", "r"}, `unknown target "nope"`},
	} {
		out, err := b.run(t, tc.stdin, append([]string{"store"}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q\n%s", tc.args, err, tc.want, out)
		}
	}
	if _, err := os.Stat(b.home); err == nil {
		entries, _ := os.ReadDir(b.home)
		if len(entries) > 0 {
			t.Errorf("a refused store wrote %v", entries)
		}
	}
}

func TestTokenInventoryAndForget(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "SECRET_ONE\n", "store", "--target", "git", "--server", "github.com", "--username", "u")
	b.ok(t, "SECRET_TWO\n", "store", "--target", "git", "--server", "gitlab.com", "--username", "u")
	out := b.ok(t, "", "inventory")
	if !strings.Contains(out, "github.com") || !strings.Contains(out, "gitlab.com") {
		t.Errorf("inventory:\n%s", out)
	}
	if strings.Contains(out, "SECRET_ONE") || strings.Contains(out, "SECRET_TWO") {
		t.Errorf("inventory printed a secret:\n%s", out)
	}
	b.ok(t, "", "forget", "git", "github.com")
	out = b.ok(t, "", "inventory")
	if !strings.Contains(out, "gitlab.com") || strings.Contains(out, "github.com") {
		t.Errorf("after forget:\n%s", out)
	}
}

func TestTokenInventoryRevealNeedsYesWithoutATerminal(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_TOPSECRET\n", "store", "--service", "github", "--server", "github.com")
	if out := b.ok(t, "", "inventory"); strings.Contains(out, "ghp_TOPSECRET") {
		t.Errorf("masked by default:\n%s", out)
	}
	out := b.ok(t, "", "inventory", "--reveal")
	if strings.Contains(out, "ghp_TOPSECRET") || !strings.Contains(out, "--yes") {
		t.Errorf("--reveal without a terminal or --yes:\n%s", out)
	}
	if out := b.ok(t, "", "inventory", "--reveal", "--yes"); !strings.Contains(out, "ghp_TOPSECRET") {
		t.Errorf("--reveal --yes:\n%s", out)
	}
}

func TestTokenInventoryRevealAsksAtATerminal(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_TTY\n", "store", "--service", "github", "--server", "github.com")
	stdinIsTerminal = func() bool { return true }
	if out := b.ok(t, "n\n", "inventory", "--reveal"); strings.Contains(out, "ghp_TTY") {
		t.Errorf("declined, still revealed:\n%s", out)
	}
	if out := b.ok(t, "y\n", "inventory", "--reveal"); !strings.Contains(out, "ghp_TTY") {
		t.Errorf("accepted, not revealed:\n%s", out)
	}
}

func TestTokenInventoryJSON(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_x\n", "store", "--service", "github", "--server", "github.com")
	items := b.inventory(t)
	if len(items) != 1 || items[0].Host != "github.com" || items[0].Secret != "" || !items[0].SecretPresent {
		t.Fatalf("inventory --json = %+v", items)
	}
	if items[0].Description == "" || items[0].File == "" {
		t.Errorf("an entry without its description or file: %+v", items[0])
	}
}

func TestTokenInventoryEmptyAndFiltered(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "inventory"); !strings.Contains(out, "No tokens installed") {
		t.Errorf("empty inventory:\n%s", out)
	}
	if out, _, _ := b.streams(t, "", "inventory", "--json"); strings.TrimSpace(out) != "[]" {
		t.Errorf("empty --json = %q, want []", out)
	}
	b.ok(t, "tok1\n", "store", "--target", "git", "--server", "github.com", "--username", "me")
	b.ok(t, "tok2\n", "store", "--target", "npm", "--server", "registry.npmjs.org")
	items := b.inventory(t, "--target", "git")
	if len(items) != 1 || items[0].Target != "git" {
		t.Errorf("--target git = %+v", items)
	}
}

func TestTokenClipCopiesWithoutPrinting(t *testing.T) {
	b := tokenSandbox(t)
	var got string
	clipboardWrite = func(s string) error { got = s; return nil }
	b.ok(t, "ghp_COPYME\n", "store", "--service", "github", "--server", "github.com")
	out := b.ok(t, "", "clip", "composer", "github.com")
	if got != "ghp_COPYME" {
		t.Errorf("clipboard got %q", got)
	}
	if strings.Contains(out, "ghp_COPYME") {
		t.Errorf("clip printed the secret:\n%s", out)
	}
	if _, err := b.run(t, "", "clip", "git", "absent.example"); err == nil {
		t.Error("clip of an absent token succeeded")
	}
}

func TestTokenForgetRefusals(t *testing.T) {
	b := tokenSandbox(t)
	if _, err := b.run(t, "", "forget", "nope", "host"); err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Errorf("unknown target: %v", err)
	}
	if _, err := b.run(t, "", "forget", "git", "absent.example"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("absent host: %v", err)
	}
}

// --- sync / skeleton / scaffold / services (test_writers.py) -------------

func TestTokenSkeletonPreviewIsRedacted(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "skeleton", "--preview")
	if !strings.Contains(out, "composer") || !strings.Contains(out, "********") {
		t.Errorf("skeleton --preview:\n%s", out)
	}
}

func TestTokenSkeletonPrintsAndWritesNothing(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "skeleton")
	if !strings.Contains(out, "<github-personal-access-token>") {
		t.Errorf("skeleton did not print the placeholders:\n%s", out)
	}
	if _, err := os.Stat(b.home); err == nil {
		if entries, _ := os.ReadDir(b.home); len(entries) > 0 {
			t.Errorf("a bare skeleton wrote %v", entries)
		}
	}
}

func TestTokenSkeletonProjectWritesOnlyProjectTargets(t *testing.T) {
	b := tokenSandbox(t)
	proj := b.project(t)
	out := b.ok(t, "", "skeleton", "--project", proj)
	for _, f := range []string{"auth.json", ".npmrc"} {
		if _, err := os.Stat(filepath.Join(proj, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
	for _, f := range []string{".docker", ".git-credentials", ".netrc"} {
		if _, err := os.Stat(filepath.Join(b.home, f)); err == nil {
			t.Errorf("placeholders reached the home's %s", f)
		}
	}
	if !strings.Contains(out, "skipped docker, git, netrc") {
		t.Errorf("no skipped note:\n%s", out)
	}
	proj2 := filepath.Join(b.root, "proj2")
	_ = os.MkdirAll(proj2, 0o700)
	b.ok(t, "", "skeleton", "--project", proj2, "--target", "composer")
	if _, err := os.Stat(filepath.Join(proj2, "auth.json")); err != nil {
		t.Errorf("--target composer: %v", err)
	}
}

func TestTokenSyncTargetWithNothingResolved(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"] = "ghp_x"
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	_, err := b.run(t, "", "sync", "--config", cfg, "--target", "docker")
	if err == nil || !strings.Contains(err.Error(), "nothing to write for docker") {
		t.Errorf("err = %v", err)
	}
}

func TestTokenSyncExportsForANonComposerTarget(t *testing.T) {
	b := tokenSandbox(t)
	b.env["NPM_TOKEN"] = "tok"
	cfg := b.config(t, `{"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	out := b.ok(t, "", "sync", "--config", cfg, "--exports")
	if strings.Contains(out, "wrote") || !strings.Contains(out, "no env channel") {
		t.Errorf("sync --exports:\n%s", out)
	}
}

func TestTokenServices(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "services"); !strings.Contains(out, "github") {
		t.Errorf("services:\n%s", out)
	}
}

func TestTokenSyncWritesProjectTargets(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"], b.env["NPM_TOKEN"] = "ghp_real", "npm_real"
	proj := b.project(t)
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}},`+
		`"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	out := b.ok(t, "", "sync", "--config", cfg, "--project", proj, "--no-env-hint")
	auth, err := os.ReadFile(filepath.Join(proj, "auth.json"))
	if err != nil || !strings.Contains(string(auth), "ghp_real") {
		t.Errorf("auth.json: %v %s", err, auth)
	}
	if _, err := os.Stat(filepath.Join(proj, ".npmrc")); err != nil {
		t.Error(err)
	}
	// Divergence from ATM: the secret is never echoed after a write.
	if strings.Contains(out, "ghp_real") || strings.Contains(out, "npm_real") {
		t.Errorf("sync printed a secret:\n%s", out)
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(filepath.Join(proj, "auth.json")); err == nil && fi.Mode().Perm() != 0o600 {
			t.Errorf("auth.json mode %o, want 600", fi.Mode().Perm())
		}
	}
}

func TestTokenSyncWithNothingResolvedFails(t *testing.T) {
	b := tokenSandbox(t)
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	out, err := b.run(t, "", "sync", "--config", cfg)
	if err == nil || !strings.Contains(err.Error(), "no credentials resolved") {
		t.Errorf("err = %v", err)
	}
	// A few warnings stay verbatim.
	if !strings.Contains(out, "skip composer:github.com") || strings.Contains(out, "skipped") {
		t.Errorf("warnings:\n%s", out)
	}
}

func TestTokenCheckup(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "checkup"); !strings.Contains(out, "composer") {
		t.Errorf("checkup with nothing written:\n%s", out)
	}
	if runtime.GOOS == "windows" {
		return
	}
	b.ok(t, "ghp_x\n", "store", "--service", "github", "--server", "github.com")
	if err := os.Chmod(b.file(t, tokens.Composer), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := b.run(t, "", "checkup", "--target", "composer")
	if err == nil || !strings.Contains(out, "WARN") {
		t.Errorf("a world-readable auth.json: err %v\n%s", err, out)
	}
}

func TestTokenSyncSourceLoadsTheEnvFile(t *testing.T) {
	b := tokenSandbox(t)
	proj := b.project(t)
	envf := filepath.Join(b.root, ".env")
	if err := os.WriteFile(envf, []byte("GH_TOKEN=test-secret-not-real\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	b.ok(t, "", "sync", "--config", cfg, "--source", envf, "--project", proj, "--no-env-hint")
	if got, _ := os.ReadFile(filepath.Join(proj, "auth.json")); !strings.Contains(string(got), "test-secret-not-real") {
		t.Errorf("auth.json = %s", got)
	}
}

func TestTokenScaffoldNeverOverwrites(t *testing.T) {
	b := tokenSandbox(t)
	_ = os.MkdirAll(b.root, 0o700)
	dest := filepath.Join(b.root, "new.json")
	b.ok(t, "", "scaffold", dest)
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := b.run(t, "", "scaffold", dest); err == nil {
		t.Error("scaffold overwrote an existing config")
	}
	one := filepath.Join(b.root, "one.json")
	out := b.ok(t, "", "scaffold", one, "--target", "composer")
	data, _ := os.ReadFile(one)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if _, ok := doc["composer"]; !ok || doc["npm"] != nil {
		t.Errorf("--target composer wrote %s (%s)", data, out)
	}
}

func TestTokenSyncExportIsOneUnwrappedLine(t *testing.T) {
	b := tokenSandbox(t)
	long := "ghp_" + strings.Repeat("x", 150)
	b.env["GH_TOKEN"] = long
	b.env["SHELL"] = "/bin/bash"
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	out := b.ok(t, "", "sync", "--config", cfg, "--exports")
	found := false
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "export COMPOSER_AUTH=") || strings.HasPrefix(l, "$env:COMPOSER_AUTH") || strings.HasPrefix(l, "set ") {
			found = strings.Contains(l, long)
		}
	}
	if !found {
		t.Errorf("no single export line with the whole token:\n%s", out)
	}
}

func TestTokenSyncPreviewHonoursTarget(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"], b.env["NPM_TOKEN"] = "ghp_x", "npm_x"
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}},`+
		`"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	out := b.ok(t, "", "sync", "--config", cfg, "--preview", "--target", "npm")
	if !strings.Contains(out, `"npm"`) || strings.Contains(out, `"composer"`) || strings.Contains(out, "npm_x") {
		t.Errorf("--preview --target npm:\n%s", out)
	}
}

func TestTokenSyncSummarisesAWallOfWarnings(t *testing.T) {
	b := tokenSandbox(t)
	b.env["NPM_OK"] = "tok"
	hosts := map[string]any{}
	for i := 0; i < 8; i++ {
		hosts["h"+string(rune('0'+i))+".example"] = map[string]string{"user_env": "U1", "pass_env": "P1"}
	}
	raw, _ := json.Marshal(map[string]any{"git": hosts, "npm": map[string]any{"registry.npmjs.org": map[string]string{"env": "NPM_OK"}}})
	cfg := b.config(t, string(raw))
	out := b.ok(t, "", "sync", "--config", cfg, "--preview")
	if !strings.Contains(out, "skipped 8 config entries") || !strings.Contains(out, "git 8") || strings.Contains(out, "h3.example") {
		t.Errorf("warning wall:\n%s", out)
	}
}

// The docker credential-store handling (security fix): a registry Docker
// keeps in a credsStore is refused with a warning unless login is chosen, and
// docker login itself is never run in dev mode.
func TestTokenStoreDockerRespectsACredentialStore(t *testing.T) {
	b := tokenSandbox(t)
	if err := os.MkdirAll(filepath.Join(b.home, ".docker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.home, ".docker", "config.json"), []byte(`{"credsStore":"desktop"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := b.ok(t, "pw\n", "store", "--target", "docker", "--server", "ghcr.io", "--username", "u")
	if strings.Contains(b.read(t, ".docker/config.json"), "auths") && strings.Contains(b.read(t, ".docker/config.json"), "ghcr.io") {
		t.Errorf("an inline auth was written despite the credential store:\n%s", b.read(t, ".docker/config.json"))
	}
	if !strings.Contains(out, "credsStore") || !strings.Contains(out, "--creds-store-mode login") {
		t.Errorf("no warning about the credential store:\n%s", out)
	}
	if _, err := b.run(t, "pw\n", "store", "--target", "docker", "--server", "ghcr.io", "--username", "u", "--creds-store-mode", "login"); err == nil {
		t.Error("docker login ran inside a dev sandbox")
	}
}

// --- shell-aliases (test_aliases.py) -------------------------------------

func TestTokenShellAliasesInstallRemoveStatus(t *testing.T) {
	b := tokenSandbox(t)
	_ = os.MkdirAll(b.root, 0o700)
	rc := filepath.Join(b.root, ".zshrc")
	const before = "export A=1\n"
	if err := os.WriteFile(rc, []byte(before), 0o640); err != nil {
		t.Fatal(err)
	}
	if out := b.ok(t, "", "shell-aliases", "--file", rc); !strings.Contains(out, "not installed") {
		t.Errorf("status before:\n%s", out)
	}
	b.ok(t, "", "shell-aliases", "--install", "--file", rc, "--quiet")
	got, _ := os.ReadFile(rc)
	if !strings.Contains(string(got), "alias atm=") || !strings.Contains(string(got), "token") {
		t.Errorf("installed:\n%s", got)
	}
	if _, err := os.Stat(rc + ".authama.bak"); err != nil {
		t.Errorf("no backup: %v", err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(rc); fi.Mode().Perm() != 0o640 {
			t.Errorf("mode %o, want the file's 640 kept", fi.Mode().Perm())
		}
	}
	if out := b.ok(t, "", "shell-aliases", "--file", rc); !strings.Contains(out, "status:  installed") {
		t.Errorf("status after install:\n%s", out)
	}
	b.ok(t, "", "shell-aliases", "--uninstall", "--file", rc, "--quiet")
	if got, _ := os.ReadFile(rc); string(got) != before {
		t.Errorf("uninstall left %q, want %q", got, before)
	}
	if out := b.ok(t, "", "shell-aliases", "--uninstall", "--file", rc); !strings.Contains(out, "no aliases installed") {
		t.Errorf("a second uninstall:\n%s", out)
	}
	if _, err := b.run(t, "", "shell-aliases", "--install", "--uninstall"); err == nil {
		t.Error("--install --uninstall together was accepted")
	}
}

func TestTokenShellAliasesPrint(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "shell-aliases", "--print")
	for _, a := range []string{"alias atm=", "alias atm-ui=", "alias atm-doctor=", "alias atm-init="} {
		if !strings.Contains(out, a) {
			t.Errorf("--print is missing %s:\n%s", a, out)
		}
	}
	if strings.Contains(out, "auth-token-manager") {
		t.Errorf("--print still names auth-token-manager:\n%s", out)
	}
}

func TestTokenShellAliasesRefusesAFileOutsideTheSandbox(t *testing.T) {
	b := tokenSandbox(t)
	outside := filepath.Join(t.TempDir(), ".bashrc")
	if _, err := b.run(t, "", "shell-aliases", "--install", "--file", outside); err == nil {
		t.Error("--dev-root installed aliases outside the sandbox")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("the outside file was created")
	}
}

// --- config discovery: legacy locations and adopt-legacy -----------------

func TestTokenSyncReadsALegacyConfigWithANotice(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"] = "ghp_x"
	legacy := filepath.Join(b.root, "legacy.json")
	_ = os.MkdirAll(b.root, 0o700)
	if err := os.WriteFile(legacy, []byte(`{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b.env["AUTH_TOKEN_MANAGER_CONFIG"] = legacy
	out := b.ok(t, "", "sync", "--preview")
	if !strings.Contains(out, "deprecated") || !strings.Contains(out, "adopt-legacy") {
		t.Errorf("no deprecation notice:\n%s", out)
	}
	if !strings.Contains(out, "github.com") {
		t.Errorf("the legacy config was not used:\n%s", out)
	}
}
