package web

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/gitsign"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/mutation"
)

// sandboxGitConfig points git's global config at a temp file, so no test
// reads or writes the real ~/.gitconfig.
func sandboxGitConfig(t *testing.T) string {
	t.Helper()
	g := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(g, []byte("[user]\n\tname = Someone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", g)
	return g
}

// /git-sign sets a profile's identity through the preview: the manifest diff
// and every line of the git files are shown, nothing is written until the
// confirmation, and apply writes the fragment, allowed_signers and the
// global block. No private key reaches the page.
func TestTheGitSignPageSetsAnIdentityThroughThePreview(t *testing.T) {
	global := sandboxGitConfig(t)
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)

	page := c.get("/git-sign")
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Set a git identity") || !strings.Contains(body, `value="git-sign-set"`) {
		t.Fatalf("/git-sign: %d\n%.1500s", page.Code, body)
	}
	if strings.Contains(body, "PRIVATE KEY") {
		t.Fatal("the page shows private key material")
	}

	dir := filepath.Join(t.TempDir(), "code", "work")
	f := url.Values{"action": {"git-sign-set"}, "profile": {"work"}, "email": {"me@work.example"},
		"user_name": {"Ada"}, "key": {""}, "sign": {"1"}, "dirs": {dir + "\n"}}
	prev := c.change("/changes/preview", f)
	pb := prev.Body.String()
	if prev.Code != http.StatusOK || !strings.Contains(pb, "config/manifest.json") || !strings.Contains(pb, "work.gitconfig") ||
		!strings.Contains(pb, global) || !strings.Contains(pb, "includeIf") {
		t.Fatalf("preview: %d\n%.2500s", prev.Code, pb)
	}
	if b, _ := os.ReadFile(global); strings.Contains(string(b), "authama") {
		t.Fatal("the preview wrote the global git config")
	}
	if _, err := os.Stat(gitsign.FragmentPath(p, "work")); err == nil {
		t.Fatal("the preview wrote the fragment")
	}
	hash := hashField.FindStringSubmatch(pb)
	if hash == nil {
		t.Fatalf("no confirm hash:\n%s", pb)
	}
	f.Set("hash", hash[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.800s", rec.Code, rec.Body.String())
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil || m.Profiles["work"].Git == nil || m.Profiles["work"].Git.Email != "me@work.example" {
		t.Fatalf("manifest: %+v %v", m.Profiles["work"].Git, err)
	}
	if b, err := os.ReadFile(gitsign.FragmentPath(p, "work")); err != nil || !strings.Contains(string(b), "me@work.example") {
		t.Fatalf("fragment: %q %v", b, err)
	}
	if b, _ := os.ReadFile(global); !strings.Contains(string(b), "includeIf") || !strings.Contains(string(b), "name = Someone") {
		t.Fatalf("global config:\n%s", b)
	}
	ops, _ := AppEngine{P: p}.History(history.Filter{Source: history.SourceWeb})
	if len(ops) != 1 || ops[0].Command != "ssh git sign set" {
		t.Errorf("recorded %+v", ops)
	}
	after := c.get("/git-sign").Body.String()
	if !strings.Contains(after, "fragment written") || !strings.Contains(after, `value="git-sign-use"`) {
		t.Errorf("the page does not show the applied identity:\n%.1500s", after)
	}
}

// Bad input is refused before a preview; use needs a full path; a checkout
// is checked and a clone worked out from GET forms.
func TestTheGitSignPageRefusesBadInputAndReads(t *testing.T) {
	sandboxGitConfig(t)
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	for name, f := range map[string]url.Values{
		"no email":      {"action": {"git-sign-set"}, "profile": {"work"}, "email": {""}},
		"bad profile":   {"action": {"git-sign-set"}, "profile": {"../x"}, "email": {"a@b.c"}},
		"relative dir":  {"action": {"git-sign-set"}, "profile": {"work"}, "email": {"a@b.c"}, "dirs": {"code/work"}},
		"relative use":  {"action": {"git-sign-use"}, "profile": {"work"}, "checkouts": {"repo"}},
		"no identity":   {"action": {"git-sign-use"}, "profile": {"work"}, "checkouts": {"/tmp"}},
		"control chars": {"action": {"git-sign-set"}, "profile": {"work"}, "email": {"a@b.c\x01"}},
	} {
		if strings.Contains(c.change("/changes/preview", f).Body.String(), `name="hash"`) {
			t.Errorf("%s: offered a confirmation", name)
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	body := c.get("/git-sign?check=" + url.QueryEscape(repo)).Body.String()
	if !strings.Contains(body, "Commits as") {
		t.Errorf("the checkout was not read:\n%.1500s", body)
	}
	body = c.get("/git-sign?repo=acme/app&mode=https").Body.String()
	if !strings.Contains(body, "https://github.com/acme/app") || !strings.Contains(body, "ssh git clone acme/app --mode https") {
		t.Errorf("the clone was not worked out:\n%.1500s", body)
	}
	if body := c.get("/git-sign?repo=nope").Body.String(); !strings.Contains(body, "want owner/repo") {
		t.Error("a bad repository was not refused")
	}
}
