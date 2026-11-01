package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/tokensvc"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/paths"
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
		"token store", tokens.NPM, "registry.npmjs.org", "@org", "SECRET_ONE",
		"token inventory", "q")
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
		t.Errorf("inventory should list the token masked:\n%s", out)
	}
}

func TestTheTUIWritesNothingWhenTheSaveIsDeclined(t *testing.T) {
	tt, buf, _ := tokenTUI(t, false, "token store", tokens.NPM, "registry.npmjs.org", "", "SECRET_ONE", "q")
	tt.run()
	if _, err := os.Stat(tt.envForTest().TargetPath(tokens.NPM, "")); err == nil {
		t.Errorf("a declined save wrote .npmrc:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "nothing changed") {
		t.Errorf("a declined save should say so:\n%s", buf.String())
	}
}

func TestTheTUIRefusesAComposerServiceWithMoreThanOneSecret(t *testing.T) {
	svc := ""
	for n, m := range tokens.LoadCatalogServices() {
		if !tokens.IsTokenMechanism(m) && m != "http-basic" {
			svc = n
			break
		}
	}
	if svc == "" {
		t.Skip("the catalog has no multi-secret service")
	}
	tt, buf, _ := tokenTUI(t, true, "token store", tokens.Composer, svc, "q")
	tt.run()
	if !strings.Contains(buf.String(), "takes more than one secret") {
		t.Errorf("want a refusal for %s:\n%s", svc, buf.String())
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

func TestTheTUIForgetsAToken(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	label := seedNPM(t, tt)
	tt.pr = &scriptPrompter{answers: []string{"token forget", label, "q"}, confirm: true}
	tt.run()
	if items := tt.envForTest().Inventory("", false); len(items) != 0 {
		t.Errorf("forget left %+v:\n%s", items, buf.String())
	}
	if !strings.Contains(buf.String(), "✓ removed "+label) {
		t.Errorf("forget should confirm:\n%s", buf.String())
	}
}

func TestTheTUICopiesATokenWithoutPrintingIt(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	label := seedNPM(t, tt)
	var got string
	clipboardWrite = func(s string) error { got = s; return nil }
	tt.pr = &scriptPrompter{answers: []string{"token clip", label, "q"}}
	tt.run()
	if got != "SECRET_TWO" {
		t.Errorf("clipboard got %q", got)
	}
	if strings.Contains(buf.String(), "SECRET_TWO") {
		t.Errorf("clip printed the secret:\n%s", buf.String())
	}
}

func TestTheTUISyncsTheBuiltInConfigAfterAPreview(t *testing.T) {
	tt, buf, b := tokenTUI(t, true, "token sync", "q")
	b.env["GH_TOKEN"] = "SECRET_ONE"
	tt.run()
	out := buf.String()
	auth := tt.envForTest().TargetPath(tokens.Composer, "")
	body, err := os.ReadFile(auth)
	if err != nil || !strings.Contains(string(body), "SECRET_ONE") {
		t.Fatalf("sync did not write %s (%v):\n%s", auth, err, out)
	}
	if strings.Contains(out, "SECRET_ONE") {
		t.Errorf("the preview showed the secret:\n%s", out)
	}
}

func TestTheTUICheckupReportsTheTokenFiles(t *testing.T) {
	tt, buf, _ := tokenTUI(t, true)
	seedNPM(t, tt)
	tt.pr = &scriptPrompter{answers: []string{"token checkup", "q"}}
	tt.run()
	if !strings.Contains(buf.String(), "npm") {
		t.Errorf("checkup should report npm:\n%s", buf.String())
	}
}

func TestTheMenuTakesTheGroupedSpellingsOnly(t *testing.T) {
	for answer, want := range map[string]string{
		"ssh list":        "list",
		"list":            "list",
		"token inventory": "token inventory",
	} {
		m, ok := lookupMenu(answer)
		if !ok || m.verb != want {
			t.Errorf("lookupMenu(%q) = %q, %v; want %q", answer, m.verb, ok, want)
		}
	}
	for _, answer := range []string{"store", "inventory", "sync"} {
		if m, ok := lookupMenu(answer); ok {
			t.Errorf("a bare %q selected %q; token entries need their group", answer, m.verb)
		}
	}
}
