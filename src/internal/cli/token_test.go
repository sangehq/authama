package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/v3/internal/core/tokens"
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
	out, errOut, err := b.streams(t, "", append([]string{"list", "--json"}, args...)...)
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

// --- set / list / delete / copy (test_cli_actions.py) -------------

func TestTokenSetGitFromStdin(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_secret\n", "set", "--target", "git", "--server", "github.com", "--username", "octocat")
	if got := b.read(t, ".git-credentials"); !strings.Contains(got, "octocat:ghp_secret@github.com") {
		t.Errorf(".git-credentials = %q", got)
	}
}

func TestTokenSetComposerViaService(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_composer\n", "set", "--service", "github", "--server", "github.com")
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

func TestTokenSetNPMWithScope(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "npmtok\n", "set", "--target", "npm", "--server", "npm.pkg.github.com", "--scope", "@org")
	got := b.read(t, ".npmrc")
	if !strings.Contains(got, "@org:registry") || !strings.Contains(got, "npmtok") {
		t.Errorf(".npmrc = %q", got)
	}
}

// ATM exited 2 for these; every failure exits 1 here (Execute), so the test
// is that each is refused and wrote nothing.
func TestTokenSetRefusesIncompleteRequests(t *testing.T) {
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
		out, err := b.run(t, tc.stdin, append([]string{"set"}, tc.args...)...)
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

func TestTokenListAndForget(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "SECRET_ONE\n", "set", "--target", "git", "--server", "github.com", "--username", "u")
	b.ok(t, "SECRET_TWO\n", "set", "--target", "git", "--server", "gitlab.com", "--username", "u")
	out := b.ok(t, "", "list")
	if !strings.Contains(out, "github.com") || !strings.Contains(out, "gitlab.com") {
		t.Errorf("inventory:\n%s", out)
	}
	if strings.Contains(out, "SECRET_ONE") || strings.Contains(out, "SECRET_TWO") {
		t.Errorf("inventory printed a secret:\n%s", out)
	}
	b.ok(t, "", "delete", "git", "github.com")
	out = b.ok(t, "", "list")
	if !strings.Contains(out, "gitlab.com") || strings.Contains(out, "github.com") {
		t.Errorf("after forget:\n%s", out)
	}
}

func TestTokenListRevealNeedsYesWithoutATerminal(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_TOPSECRET\n", "set", "--service", "github", "--server", "github.com")
	if out := b.ok(t, "", "list"); strings.Contains(out, "ghp_TOPSECRET") {
		t.Errorf("masked by default:\n%s", out)
	}
	out := b.ok(t, "", "list", "--reveal")
	if strings.Contains(out, "ghp_TOPSECRET") || !strings.Contains(out, "--yes") {
		t.Errorf("--reveal without a terminal or --yes:\n%s", out)
	}
	if out := b.ok(t, "", "list", "--reveal", "--yes"); !strings.Contains(out, "ghp_TOPSECRET") {
		t.Errorf("--reveal --yes:\n%s", out)
	}
}

func TestTokenListRevealAsksAtATerminal(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_TTY\n", "set", "--service", "github", "--server", "github.com")
	stdinIsTerminal = func() bool { return true }
	if out := b.ok(t, "n\n", "list", "--reveal"); strings.Contains(out, "ghp_TTY") {
		t.Errorf("declined, still revealed:\n%s", out)
	}
	if out := b.ok(t, "y\n", "list", "--reveal"); !strings.Contains(out, "ghp_TTY") {
		t.Errorf("accepted, not revealed:\n%s", out)
	}
}

func TestTokenListJSON(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "ghp_x\n", "set", "--service", "github", "--server", "github.com")
	items := b.inventory(t)
	if len(items) != 1 || items[0].Host != "github.com" || items[0].Secret != "" || !items[0].SecretPresent {
		t.Fatalf("inventory --json = %+v", items)
	}
	if items[0].Description == "" || items[0].File == "" {
		t.Errorf("an entry without its description or file: %+v", items[0])
	}
}

func TestTokenListEmptyAndFiltered(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "list"); !strings.Contains(out, "No tokens installed") {
		t.Errorf("empty inventory:\n%s", out)
	}
	if out, _, _ := b.streams(t, "", "list", "--json"); strings.TrimSpace(out) != "[]" {
		t.Errorf("empty --json = %q, want []", out)
	}
	b.ok(t, "tok1\n", "set", "--target", "git", "--server", "github.com", "--username", "me")
	b.ok(t, "tok2\n", "set", "--target", "npm", "--server", "registry.npmjs.org")
	items := b.inventory(t, "--target", "git")
	if len(items) != 1 || items[0].Target != "git" {
		t.Errorf("--target git = %+v", items)
	}
}

func TestTokenCopyCopiesWithoutPrinting(t *testing.T) {
	b := tokenSandbox(t)
	var got string
	clipboardWrite = func(s string) error { got = s; return nil }
	b.ok(t, "ghp_COPYME\n", "set", "--service", "github", "--server", "github.com")
	out := b.ok(t, "", "copy", "composer", "github.com")
	if got != "ghp_COPYME" {
		t.Errorf("clipboard got %q", got)
	}
	if strings.Contains(out, "ghp_COPYME") {
		t.Errorf("clip printed the secret:\n%s", out)
	}
	if _, err := b.run(t, "", "copy", "git", "absent.example"); err == nil {
		t.Error("clip of an absent token succeeded")
	}
}

func TestTokenDeleteRefusals(t *testing.T) {
	b := tokenSandbox(t)
	if _, err := b.run(t, "", "delete", "nope", "host"); err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Errorf("unknown target: %v", err)
	}
	if _, err := b.run(t, "", "delete", "git", "absent.example"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("absent host: %v", err)
	}
}

// --- apply / examples / init / providers (test_writers.py) -------------

func TestTokenExamplesPreviewIsRedacted(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "examples", "--json")
	if !strings.Contains(out, "composer") || !strings.Contains(out, "********") {
		t.Errorf("examples --json:\n%s", out)
	}
}

func TestTokenExamplesPrintsAndWritesNothing(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "examples")
	if !strings.Contains(out, "<github-personal-access-token>") {
		t.Errorf("skeleton did not print the placeholders:\n%s", out)
	}
	if _, err := os.Stat(b.home); err == nil {
		if entries, _ := os.ReadDir(b.home); len(entries) > 0 {
			t.Errorf("a bare skeleton wrote %v", entries)
		}
	}
}

func TestTokenExamplesProjectWritesOnlyProjectTargets(t *testing.T) {
	b := tokenSandbox(t)
	proj := b.project(t)
	out := b.ok(t, "", "examples", "--project", proj)
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
	b.ok(t, "", "examples", "--project", proj2, "--target", "composer")
	if _, err := os.Stat(filepath.Join(proj2, "auth.json")); err != nil {
		t.Errorf("--target composer: %v", err)
	}
}

func TestTokenApplyTargetWithNothingResolved(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"] = "ghp_x"
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	_, err := b.run(t, "", "apply", "--config", cfg, "--target", "docker")
	if err == nil || !strings.Contains(err.Error(), "nothing to write for docker") {
		t.Errorf("err = %v", err)
	}
}

func TestTokenApplyExportsForANonComposerTarget(t *testing.T) {
	b := tokenSandbox(t)
	b.env["NPM_TOKEN"] = "tok"
	cfg := b.config(t, `{"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	out := b.ok(t, "", "apply", "--config", cfg, "--exports")
	if strings.Contains(out, "wrote") || !strings.Contains(out, "no env channel") {
		t.Errorf("sync --exports:\n%s", out)
	}
}

func TestTokenProviders(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "providers"); !strings.Contains(out, "github") {
		t.Errorf("services:\n%s", out)
	}
}

func TestTokenApplyWritesProjectTargets(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"], b.env["NPM_TOKEN"] = "ghp_real", "npm_real"
	proj := b.project(t)
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}},`+
		`"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	out := b.ok(t, "", "apply", "--config", cfg, "--project", proj, "--no-env-hint")
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

func TestTokenApplyWithNothingResolvedFails(t *testing.T) {
	b := tokenSandbox(t)
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	out, err := b.run(t, "", "apply", "--config", cfg)
	if err == nil || !strings.Contains(err.Error(), "no credentials resolved") {
		t.Errorf("err = %v", err)
	}
	// A few warnings stay verbatim.
	if !strings.Contains(out, "skip composer:github.com") || strings.Contains(out, "skipped") {
		t.Errorf("warnings:\n%s", out)
	}
}

func TestTokenAudit(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "audit"); !strings.Contains(out, "composer") {
		t.Errorf("checkup with nothing written:\n%s", out)
	}
	if runtime.GOOS == "windows" {
		return
	}
	b.ok(t, "ghp_x\n", "set", "--service", "github", "--server", "github.com")
	if err := os.Chmod(b.file(t, tokens.Composer), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := b.run(t, "", "audit", "--target", "composer")
	if err == nil || !strings.Contains(out, "WARN") {
		t.Errorf("a world-readable auth.json: err %v\n%s", err, out)
	}
}

func TestTokenApplySourceLoadsTheEnvFile(t *testing.T) {
	b := tokenSandbox(t)
	proj := b.project(t)
	envf := filepath.Join(b.root, ".env")
	if err := os.WriteFile(envf, []byte("GH_TOKEN=test-secret-not-real\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	b.ok(t, "", "apply", "--config", cfg, "--source", envf, "--project", proj, "--no-env-hint")
	if got, _ := os.ReadFile(filepath.Join(proj, "auth.json")); !strings.Contains(string(got), "test-secret-not-real") {
		t.Errorf("auth.json = %s", got)
	}
}

func TestTokenInitNeverOverwrites(t *testing.T) {
	b := tokenSandbox(t)
	_ = os.MkdirAll(b.root, 0o700)
	dest := filepath.Join(b.root, "new.json")
	b.ok(t, "", "init", dest)
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
	if _, err := b.run(t, "", "init", dest); err == nil {
		t.Error("scaffold overwrote an existing config")
	}
	one := filepath.Join(b.root, "one.json")
	out := b.ok(t, "", "init", one, "--target", "composer")
	data, _ := os.ReadFile(one)
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if _, ok := doc["composer"]; !ok || doc["npm"] != nil {
		t.Errorf("--target composer wrote %s (%s)", data, out)
	}
}

func TestTokenApplyExportIsOneUnwrappedLine(t *testing.T) {
	b := tokenSandbox(t)
	long := "ghp_" + strings.Repeat("x", 150)
	b.env["GH_TOKEN"] = long
	b.env["SHELL"] = "/bin/bash"
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`)
	out := b.ok(t, "", "apply", "--config", cfg, "--exports")
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

func TestTokenApplyPreviewHonoursTarget(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"], b.env["NPM_TOKEN"] = "ghp_x", "npm_x"
	cfg := b.config(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}},`+
		`"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	out := b.ok(t, "", "apply", "--config", cfg, "--dry-run", "--target", "npm")
	if !strings.Contains(out, `"npm"`) || strings.Contains(out, `"composer"`) || strings.Contains(out, "npm_x") {
		t.Errorf("--dry-run --target npm:\n%s", out)
	}
}

func TestTokenApplySummarisesAWallOfWarnings(t *testing.T) {
	b := tokenSandbox(t)
	b.env["NPM_OK"] = "tok"
	hosts := map[string]any{}
	for i := 0; i < 8; i++ {
		hosts["h"+string(rune('0'+i))+".example"] = map[string]string{"user_env": "U1", "pass_env": "P1"}
	}
	raw, _ := json.Marshal(map[string]any{"git": hosts, "npm": map[string]any{"registry.npmjs.org": map[string]string{"env": "NPM_OK"}}})
	cfg := b.config(t, string(raw))
	out := b.ok(t, "", "apply", "--config", cfg, "--dry-run")
	if !strings.Contains(out, "skipped 8 config entries") || !strings.Contains(out, "git 8") || strings.Contains(out, "h3.example") {
		t.Errorf("warning wall:\n%s", out)
	}
}

// The docker credential-store handling (security fix): a registry Docker
// keeps in a credsStore is refused with a warning unless login is chosen, and
// docker login itself is never run in dev mode.
func TestTokenSetDockerRespectsACredentialStore(t *testing.T) {
	b := tokenSandbox(t)
	if err := os.MkdirAll(filepath.Join(b.home, ".docker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.home, ".docker", "config.json"), []byte(`{"credsStore":"desktop"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := b.ok(t, "pw\n", "set", "--target", "docker", "--server", "ghcr.io", "--username", "u")
	if strings.Contains(b.read(t, ".docker/config.json"), "auths") && strings.Contains(b.read(t, ".docker/config.json"), "ghcr.io") {
		t.Errorf("an inline auth was written despite the credential store:\n%s", b.read(t, ".docker/config.json"))
	}
	if !strings.Contains(out, "credsStore") || !strings.Contains(out, "--creds-store-mode login") {
		t.Errorf("no warning about the credential store:\n%s", out)
	}
	if _, err := b.run(t, "pw\n", "set", "--target", "docker", "--server", "ghcr.io", "--username", "u", "--creds-store-mode", "login"); err == nil {
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

// --- config discovery: legacy locations and import -----------------

func TestTokenApplyReadsALegacyConfigWithANotice(t *testing.T) {
	b := tokenSandbox(t)
	b.env["GH_TOKEN"] = "ghp_x"
	legacy := filepath.Join(b.root, "legacy.json")
	_ = os.MkdirAll(b.root, 0o700)
	if err := os.WriteFile(legacy, []byte(`{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b.env["AUTH_TOKEN_MANAGER_CONFIG"] = legacy
	out := b.ok(t, "", "apply", "--dry-run")
	if !strings.Contains(out, "deprecated") || !strings.Contains(out, "import") {
		t.Errorf("no deprecation notice:\n%s", out)
	}
	if !strings.Contains(out, "github.com") {
		t.Errorf("the legacy config was not used:\n%s", out)
	}
}

// --- the doctor's token section ------------------------------------------

func TestDoctorChecksAndFixesTokenFilePerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	b := tokenSandbox(t)
	npmrc := filepath.Join(b.home, ".npmrc")
	if err := os.MkdirAll(b.home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(npmrc, []byte("//registry.npmjs.org/:_authToken=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doctor := func(args ...string) (string, string, error) {
		root := newRootCmd()
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs(append([]string{"--dev-root", b.root, "doctor"}, args...))
		err := root.Execute()
		return out.String(), errOut.String(), err
	}
	// The exit status the SSH half of the sandbox earns on its own, with the
	// token file owner-only: a loose token file must not change it.
	_ = os.Chmod(npmrc, 0o600)
	_, _, baseline := doctor()
	_ = os.Chmod(npmrc, 0o644)
	out, _, err := doctor()
	if !strings.Contains(out, "=== auth tokens ===") || !strings.Contains(out, "✗ npm: "+npmrc) {
		t.Errorf("doctor did not flag a group-readable .npmrc:\n%s", out)
	}
	if (err == nil) != (baseline == nil) {
		t.Errorf("a loose token file changed the doctor's exit (%v, was %v): it warns unless --strict", err, baseline)
	}
	if !strings.Contains(out, "--strict makes it fail") {
		t.Errorf("the warning does not say how to make it fatal:\n%s", out)
	}
	if _, _, err := doctor("--strict"); !errors.Is(err, errNotClean) {
		t.Errorf("doctor --strict passed with a group-readable .npmrc: %v", err)
	}
	js, _, _ := doctor("--json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatalf("doctor --json no longer parses: %v\n%s", err, js)
	}
	tok, _ := doc["tokens"].(map[string]any)
	if tok == nil || tok["ok"] != false {
		t.Errorf("doctor --json tokens = %v", doc["tokens"])
	}
	if _, ok := doc["perm_issues"]; !ok {
		t.Error("the SSH report's keys are gone from doctor --json")
	}
	out, _, _ = doctor("--fix")
	if fi, _ := os.Stat(npmrc); fi.Mode().Perm() != 0o600 {
		t.Errorf("doctor --fix left .npmrc at %o:\n%s", fi.Mode().Perm(), out)
	}
	if !strings.Contains(out, "✓ npm: "+npmrc) {
		t.Errorf("after --fix:\n%s", out)
	}
}

// --- show / status / import / delete's question (the commands a complete
// token manager has beyond auth-token-manager's own) ----------------------

func TestTokenShowOneEntryMaskedUnlessRevealed(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "SECRET_SHOW\n", "set", "--target", "git", "--server", "github.com", "--username", "octocat")
	out := b.ok(t, "", "show", "git", "https://github.com/")
	for _, want := range []string{"tool:", "git", "host:", "github.com", "octocat", "••••••••", "file:"} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "SECRET_SHOW") {
		t.Fatalf("show printed the secret:\n%s", out)
	}
	if out := b.ok(t, "", "show", "git", "github.com", "--reveal"); strings.Contains(out, "SECRET_SHOW") {
		t.Errorf("--reveal without a terminal or --yes printed the secret:\n%s", out)
	}
	if out := b.ok(t, "", "show", "git", "github.com", "--reveal", "--yes"); !strings.Contains(out, "SECRET_SHOW") {
		t.Errorf("--reveal --yes did not reveal:\n%s", out)
	}
	js, _, err := b.streams(t, "", "show", "git", "github.com", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var it tokens.Installed
	if err := json.Unmarshal([]byte(js), &it); err != nil || it.Username != "octocat" || it.Secret != "" || !it.SecretPresent {
		t.Errorf("show --json = %+v (%v)\n%s", it, err, js)
	}
	if _, err := b.run(t, "", "show", "git", "absent.example"); err == nil || !strings.Contains(err.Error(), "token list") {
		t.Errorf("an absent entry: %v", err)
	}
}

func TestTokenStatusReportsConfigFilesAndMissingVariables(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "status")
	if !strings.Contains(out, "built-in default") || !strings.Contains(out, "absent") {
		t.Errorf("status with nothing set up:\n%s", out)
	}
	if !strings.Contains(out, "skip composer:github.com") {
		t.Errorf("status does not say the default entry is skipped for an unset GH_TOKEN:\n%s", out)
	}
	cfg := b.config(t, `{"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}},"git":{"github.com":{"user_env":"GU","pass_env":"GP"}}}`)
	b.env["NPM_TOKEN"] = "npm_x"
	b.ok(t, "tok\n", "set", "--target", "npm", "--server", "registry.npmjs.org")
	js, _, err := b.streams(t, "", "status", "--config", cfg, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var st tokenStatus
	if err := json.Unmarshal([]byte(js), &st); err != nil {
		t.Fatalf("%v\n%s", err, js)
	}
	if st.Config.Path != cfg || st.Config.Builtin || len(st.Targets) != len(tokens.Targets()) {
		t.Fatalf("status --json = %+v", st)
	}
	for _, r := range st.Targets {
		switch r.Target {
		case tokens.NPM:
			if !r.Exists || r.Installed != 1 || r.Configured != 1 || r.Resolved != 1 {
				t.Errorf("npm row = %+v", r)
			}
			if runtime.GOOS != "windows" && r.Mode != "0600" {
				t.Errorf("npm mode = %s", r.Mode)
			}
		case tokens.Git:
			if r.Configured != 1 || r.Resolved != 0 {
				t.Errorf("git row = %+v", r)
			}
		}
	}
	if len(st.Warnings) != 1 || !strings.Contains(st.Warnings[0], "git:github.com") {
		t.Errorf("warnings = %q", st.Warnings)
	}
	if strings.Contains(js, "npm_x") || strings.Contains(js, `"tok"`) {
		t.Errorf("status leaked a secret:\n%s", js)
	}
}

func TestTokenProvidersListsServicesAndHosts(t *testing.T) {
	b := tokenSandbox(t)
	out := b.ok(t, "", "providers")
	for _, want := range []string{"SERVICE", "github", "http-basic", "ghcr.io", "registry.npmjs.org", "dev.azure.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("providers lacks %q:\n%s", want, out)
		}
	}
}

func TestTokenImportFromALegacyLocationOrAFile(t *testing.T) {
	b := tokenSandbox(t)
	if out := b.ok(t, "", "import"); !strings.Contains(out, "nothing to import") {
		t.Errorf("import with nothing to import:\n%s", out)
	}
	_ = os.MkdirAll(b.root, 0o700)
	team := filepath.Join(b.root, "team.json")
	if err := os.WriteFile(team, []byte(`{"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := b.ok(t, "", "import", team, "--dry-run")
	if !strings.Contains(out, "would copy") {
		t.Errorf("--dry-run:\n%s", out)
	}
	if out := b.ok(t, "", "status"); !strings.Contains(out, "built-in default") {
		t.Errorf("--dry-run wrote the config:\n%s", out)
	}
	if out := b.ok(t, "", "import", team); !strings.Contains(out, "✓ copied") {
		t.Errorf("import FILE:\n%s", out)
	}
	if _, err := os.Stat(team); err != nil {
		t.Error("import removed the original")
	}
	if _, err := b.run(t, "", "import", team); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("a second import overwrote tokens.json: %v", err)
	}
	broken := filepath.Join(b.root, "broken.json")
	_ = os.WriteFile(broken, []byte(`{not json`), 0o600)
	b2 := tokenSandbox(t)
	if _, err := b2.run(t, "", "import", broken); err == nil {
		t.Error("a config that does not parse was imported")
	}
}

func TestTokenDeleteAsksAtATerminal(t *testing.T) {
	b := tokenSandbox(t)
	b.ok(t, "SECRET_DEL\n", "set", "--target", "npm", "--server", "registry.npmjs.org")
	stdinIsTerminal = func() bool { return true }
	if out := b.ok(t, "n\n", "delete", "npm", "registry.npmjs.org"); !strings.Contains(out, "Kept it") {
		t.Errorf("declined:\n%s", out)
	}
	if len(b.inventory(t)) != 1 {
		t.Fatal("a declined delete removed the token")
	}
	b.ok(t, "", "delete", "npm", "registry.npmjs.org", "--yes")
	if len(b.inventory(t)) != 0 {
		t.Error("--yes did not delete")
	}
}

// auth-token-manager read its catalog from $AUTH_TOKEN_MANAGER_CATALOG when
// set, so a Composer service could be added without a release. authama reads
// $SSH_MANAGER_TOKENS_CATALOG, and still the old name with a notice.
func TestTokenCatalogOverride(t *testing.T) {
	b := tokenSandbox(t)
	t.Cleanup(func() { _ = tokens.UseCatalog("") })
	embedded, err := tokens.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(embeddedCatalogText(t)), &doc); err != nil {
		t.Fatal(err)
	}
	doc["composer"].(map[string]any)["services"].(map[string]any)["acme-forge"] = "gitlab-token"
	custom, _ := json.Marshal(doc)
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, custom, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, v := range []string{"SSH_MANAGER_TOKENS_CATALOG", "AUTH_TOKEN_MANAGER_CATALOG"} {
		b.env = map[string]string{v: path}
		out := b.ok(t, "", "providers")
		if !strings.Contains(out, "acme-forge") {
			t.Errorf("$%s: the custom service is not listed:\n%s", v, out)
		}
		if legacy := v == "AUTH_TOKEN_MANAGER_CATALOG"; legacy != strings.Contains(out, "deprecated:") {
			t.Errorf("$%s: deprecation notice wrong:\n%s", v, out)
		}
	}

	// A missing file warns and falls back, as auth-token-manager did.
	b.env = map[string]string{"SSH_MANAGER_TOKENS_CATALOG": filepath.Join(t.TempDir(), "nope.json")}
	out := b.ok(t, "", "providers")
	if !strings.Contains(out, "not a file") || strings.Contains(out, "acme-forge") {
		t.Errorf("missing catalog file:\n%s", out)
	}
	if got, _ := tokens.LoadCatalog(); got != embedded {
		t.Error("the embedded catalog was not restored")
	}

	// A broken file is refused before anything runs.
	if err := os.WriteFile(path, []byte(`{"targets":["composer","pip"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b.env = map[string]string{"SSH_MANAGER_TOKENS_CATALOG": path}
	if out, err := b.run(t, "", "providers"); err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Errorf("a catalog naming an unknown target was used: %v\n%s", err, out)
	}

	// Unset, the embedded catalog is back.
	b.env = map[string]string{}
	if out := b.ok(t, "", "providers"); strings.Contains(out, "acme-forge") {
		t.Errorf("the override outlived its variable:\n%s", out)
	}
}

func embeddedCatalogText(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "core", "tokens", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
