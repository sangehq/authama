package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// /import offers this machine's ssh configs by name. The form carries the
// name; the path is shown, never posted, so no path arrives from a browser.
func TestImportOffersConfigsByName(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if !strings.Contains(c.get("/profiles").Body.String(), `/import"`) {
		t.Error("the Profiles page does not link to /import")
	}
	body := c.get("/import").Body.String()
	cfg := filepath.Join(p.SSHDir, "config")
	for _, want := range []string{`value="ssh-import"`, `name="source" value="ssh"`, cfg, `name="replace"`, "--force"} {
		if !strings.Contains(body, want) {
			t.Errorf("/import has no %q", want)
		}
	}
	if strings.Contains(body, `value="`+cfg+`"`) {
		t.Error("/import posts a path")
	}
	if strings.Contains(body, `value="config-home"`) {
		t.Error("/import offers a config home that has no ssh/config")
	}

	// A config home with an ssh/config is offered too.
	home := os.Getenv("XDG_CONFIG_HOME")
	if err := os.MkdirAll(filepath.Join(home, "ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "ssh", "config"), []byte("Host other\n  HostName other.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.get("/import").Body.String(), `name="source" value="config-home"`) {
		t.Error("/import does not offer the config home's ssh/config")
	}
}

// Previewing an import is `ssh import <config> --force --yes`, for a source
// this machine has; any other name, or a path, is refused before anything
// is read.
func TestImportFromThePageIsTheCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	rec := c.change("/changes/preview", url.Values{"action": {"ssh-import"}, "source": {"ssh"}, "replace": {"1"}})
	want := "authama ssh import " + filepath.Join(p.SSHDir, "config") + " --force --yes"
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("%d, want %q\n%s", rec.Code, want, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "import-backup-") {
		t.Error("the preview does not say the manifest is backed up first")
	}
	for _, src := range []string{"", "nope", "config-home", "/etc/ssh/ssh_config", "../.ssh/config", filepath.Join(p.SSHDir, "config")} {
		f := url.Values{"action": {"ssh-import"}, "source": {src}, "replace": {"1"}}
		if rec := c.change("/changes/preview", f); rec.Code != http.StatusBadRequest {
			t.Errorf("source %q was accepted: %d", src, rec.Code)
		}
	}
}

// A read-only server shows where import would read from, and offers nothing.
func TestReadOnlyImportOffersNoChange(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	if body := c.get("/import").Body.String(); strings.Contains(body, "ssh-import") {
		t.Error("a read-only /import offers an import")
	}
	if strings.Contains(c.get("/profiles").Body.String(), `/import"`) {
		t.Error("a read-only Profiles page links to import")
	}
}
