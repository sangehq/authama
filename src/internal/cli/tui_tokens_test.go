package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/v3/internal/core/tokens"
	"github.com/simtabi/authama/src/v3/internal/services/tokensvc"
	"github.com/simtabi/authama/src/v3/internal/util/paths"
)

// tokenTUI is a menu over a --dev-root sandbox, so every token file the
// session writes lands under the sandbox's home and never the real one.
func tokenTUI(t *testing.T, confirm bool, answers ...string) (*tui, *bytes.Buffer, *tokenBox) {
	t.Helper()
	b := tokenSandbox(t)
	releaseHeldLock(t)
	cfg := filepath.Join(b.root, "config")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{SSHDir: filepath.Join(b.root, "ssh"), ConfigDir: cfg, DevRoot: b.root}
	var buf bytes.Buffer
	return &tui{p: p, pr: &scriptPrompter{answers: answers, confirm: confirm}, out: &buf}, &buf, b
}

func (t *tui) envForTest() tokensvc.Env { return tokenMenu{t}.tokenEnv() }

func TestTheTUIStoresATokenInTheSandbox(t *testing.T) {
	tt, buf, b := tokenTUI(t, true,
		"token set", tokens.NPM, "npm public registry  (registry.npmjs.org)", "@org", "SECRET_ONE",
		"token list", "q")
	tt.run()
	out := buf.String()
	npmrc := tt.envForTest().TargetPath(tokens.NPM, "")
	if !strings.HasPrefix(npmrc, b.root) {
		t.Fatalf("the TUI resolved .npmrc outside the sandbox: %s", npmrc)
	}
	body, err := os.ReadFile(npmrc)
	if err != nil || !strings.Contains(string(body), "SECRET_ONE") || !strings.Contains(string(body), "@org:registry") {
		t.Fatalf("store did not write .npmrc (%v):\n%s\n--- screen\n%s", err, body, out)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(npmrc); fi.Mode().Perm() != 0o600 {
			t.Errorf(".npmrc mode %o, want 600", fi.Mode().Perm())
		}
	}
	if strings.Contains(out, "SECRET_ONE") {
		t.Errorf("the secret reached the screen:\n%s", out)
	}
	if !strings.Contains(out, "TOOL") || !strings.Contains(out, "••••••••") {
		t.Errorf("list should show the token masked:\n%s", out)
	}
}

func TestTheTUIWritesNothingWhenTheSaveIsDeclined(t *testing.T) {
	tt, buf, _ := tokenTUI(t, false, "token set", tokens.NPM, "npm public registry  (registry.npmjs.org)", "", "SECRET_ONE", "q")
	tt.run()
	if _, err := os.Stat(tt.envForTest().TargetPath(tokens.NPM, "")); err == nil {
		t.Errorf("a declined save wrote .npmrc:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "nothing changed") {
		t.Errorf("a declined save should say so:\n%s", buf.String())
	}
}

func readTokenFile(t *testing.T, tt *tui, target string) string {
	t.Helper()
	body, err := os.ReadFile(tt.envForTest().TargetPath(target, ""))
	if err != nil {
		t.Fatalf("%s was not written: %v", target, err)
	}
	return string(body)
}

// auth-token-manager's menu stored the Composer mechanisms its CLI refused:
// a Bitbucket consumer key and secret, and custom headers.
func TestTheTUIStoresComposerPairsAndHeaders(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true,
		"token set", tokens.Composer, "bitbucket  (bitbucket.org)", "Cloud  (bitbucket.org)", "KEY_ONE", "SECRET_PAIR", "q")
	tt.run()
	if body := readTokenFile(t, tt, tokens.Composer); !strings.Contains(body, "bitbucket-oauth") ||
		!strings.Contains(body, "KEY_ONE") || !strings.Contains(body, "SECRET_PAIR") {
		t.Fatalf("bitbucket-oauth not stored:\n%s\n--- screen\n%s", body, buf.String())
	}
	tt2, buf2, _ := tokenTUI(t, true,
		"token set", tokens.Composer, "generic-headers", "repo.acme.test", "API-TOKEN: SECRET_H, X-Team: a", "q")
	tt2.run()
	if body := readTokenFile(t, tt2, tokens.Composer); !strings.Contains(body, "custom-headers") ||
		!strings.Contains(body, "API-TOKEN: SECRET_H") || !strings.Contains(body, "X-Team: a") {
		t.Fatalf("custom-headers not stored:\n%s\n--- screen\n%s", body, buf2.String())
	}
}

// A self-hosted GitHub needs its host in composer.json's github-domains, or
// Composer never uses the token; the menu says so, as auth-token-manager did.
func TestTheTUIRemindsAboutSelfHostedComposerDomains(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true,
		"token set", tokens.Composer, "github  (github.com)", "Self-hosted / Enterprise (custom URL)",
		"https://git.acme.test/", "SECRET_GHE", "q")
	tt.run()
	if body := readTokenFile(t, tt, tokens.Composer); !strings.Contains(body, `"git.acme.test"`) {
		t.Fatalf("the typed URL was not trimmed to a host:\n%s", body)
	}
	if !strings.Contains(buf.String(), `add git.acme.test to "github-domains"`) {
		t.Errorf("want the github-domains reminder:\n%s", buf.String())
	}
}

func TestTheTUIAsksForAHostWhenTheProviderHasNoCloud(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true,
		"token set", tokens.Git, "Gitea / Forgejo / Codeberg", "http://gitea.acme.test/", "me", "SECRET_G", "q")
	tt.run()
	if body := readTokenFile(t, tt, tokens.Git); !strings.Contains(body, "me:SECRET_G@gitea.acme.test") {
		t.Fatalf("git credential not stored for the typed host:\n%s\n--- screen\n%s", body, buf.String())
	}
}

func seedNPM(t *testing.T, tt *tui) string {
	t.Helper()
	env := tt.envForTest()
	set := tokens.NewSet()
	set.Apply(tokens.NPM, "", "registry.npmjs.org", tokens.Credential{Token: "SECRET_TWO"})
	if _, err := env.WriteTarget(set, tokens.NPM, env.TargetPath(tokens.NPM, "")); err != nil {
		t.Fatal(err)
	}
	items := env.Inventory("", false)
	if len(items) != 1 {
		t.Fatalf("seeded inventory = %+v", items)
	}
	return items[0].Target + ": " + items[0].Host
}

// Replacing an installed entry keeps its tool, host and user name (Enter
// keeps it) and asks only for the new secret: auth-token-manager's "Edit".
func TestTheTUIReplacesAnInstalledToken(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	set := tokens.NewSet()
	set.Apply(tokens.Git, "", "github.com", tokens.Credential{Username: "me", Password: "SECRET_OLD"})
	env := tt.envForTest()
	if _, err := env.WriteTarget(set, tokens.Git, env.TargetPath(tokens.Git, "")); err != nil {
		t.Fatal(err)
	}
	tt.pr = &scriptPrompter{answers: []string{"token set", "Replace git: github.com (me)", "", "SECRET_NEW", "q"}, confirm: true}
	tt.run()
	body := readTokenFile(t, tt, tokens.Git)
	if !strings.Contains(body, "me:SECRET_NEW@github.com") || strings.Contains(body, "SECRET_OLD") {
		t.Fatalf("replace did not rotate the secret:\n%s\n--- screen\n%s", body, buf.String())
	}
}

func TestTheTUIDeletesAToken(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	label := seedNPM(t, tt)
	tt.pr = &scriptPrompter{answers: []string{"token delete", label, "q"}, confirm: true}
	tt.run()
	if items := tt.envForTest().Inventory("", false); len(items) != 0 {
		t.Errorf("delete left %+v:\n%s", items, buf.String())
	}
	if !strings.Contains(buf.String(), "✓ removed "+label) {
		t.Errorf("delete should confirm:\n%s", buf.String())
	}
}

func TestTheTUICopiesATokenWithoutPrintingIt(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	label := seedNPM(t, tt)
	var got string
	clipboardWrite = func(s string) error { got = s; return nil }
	tt.pr = &scriptPrompter{answers: []string{"token copy", label, "q"}}
	tt.run()
	if got != "SECRET_TWO" {
		t.Errorf("clipboard got %q", got)
	}
	if strings.Contains(buf.String(), "SECRET_TWO") {
		t.Errorf("clip printed the secret:\n%s", buf.String())
	}
}

func TestTheTUISyncsTheBuiltInConfigAfterAPreview(t *testing.T) {
	tt, buf, b := tokenTUI(t, true, "token apply", "q")
	b.env["GH_TOKEN"] = "SECRET_ONE"
	tt.run()
	out := buf.String()
	auth := tt.envForTest().TargetPath(tokens.Composer, "")
	body, err := os.ReadFile(auth)
	if err != nil || !strings.Contains(string(body), "SECRET_ONE") {
		t.Fatalf("apply did not write %s (%v):\n%s", auth, err, out)
	}
	if strings.Contains(out, "SECRET_ONE") {
		t.Errorf("the preview showed the secret:\n%s", out)
	}
}

func TestTheTUIAuditReportsTheTokenFiles(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	seedNPM(t, tt)
	tt.pr = &scriptPrompter{answers: []string{"token audit", "q"}}
	tt.run()
	if !strings.Contains(buf.String(), "npm") {
		t.Errorf("audit should report npm:\n%s", buf.String())
	}
}

func TestTheMenuTakesTheGroupedSpellingsOnly(t *testing.T) {
	for answer, want := range map[string]string{
		"ssh list":   "list",
		"list":       "list",
		"token list": "token list",
	} {
		m, ok := lookupMenu(answer)
		if !ok || m.verb != want {
			t.Errorf("lookupMenu(%q) = %q, %v; want %q", answer, m.verb, ok, want)
		}
	}
	for _, answer := range []string{"set", "copy", "apply"} {
		if m, ok := lookupMenu(answer); ok {
			t.Errorf("a bare %q selected %q; token entries need their group", answer, m.verb)
		}
	}
}
