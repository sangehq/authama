package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/gitsign"
)

func TestGitSignSetAppliesAndClearsTheProfilesIdentity(t *testing.T) {
	p := editHome(t)
	global := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "github-work", "-H", "github.com", "-u", "git")

	out := run(t, "ssh", "git", "sign", "set", "work", "--email", "ada@acme.example", "--name", "Ada", "--dir", "/srv/acme", "-y")
	if !strings.Contains(out, "set the git identity of profile work") || !strings.Contains(out, "wrote "+global) {
		t.Fatalf("set:\n%s", out)
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	if g := m.Profiles["work"].Git; g == nil || g.Email != "ada@acme.example" || !g.Sign || g.Dirs[0] != "/srv/acme" {
		t.Fatalf("manifest: %+v", g)
	}
	cfg, _ := os.ReadFile(global)
	if !strings.Contains(string(cfg), `gitdir:/srv/acme/`) {
		t.Errorf("global config:\n%s", cfg)
	}
	frag, _ := os.ReadFile(gitsign.FragmentPath(p, "work"))
	if !strings.Contains(string(frag), "email = ada@acme.example") || strings.Contains(string(frag), "gpgsign") {
		t.Errorf("a key not made yet must not turn signing on:\n%s", frag)
	}

	out = run(t, "ssh", "git", "sign", "status")
	if !strings.Contains(out, "work") || !strings.Contains(out, "signing waits for work/") || !strings.Contains(out, "applied: yes") {
		t.Errorf("status:\n%s", out)
	}
	if out := run(t, "ssh", "git", "sign", "apply", "--dry-run"); !strings.Contains(out, "up to date") {
		t.Errorf("a second apply:\n%s", out)
	}

	out = run(t, "ssh", "git", "sign", "set", "work", "--clear", "-y")
	if !strings.Contains(out, "cleared") {
		t.Fatalf("clear:\n%s", out)
	}
	if cfg, _ := os.ReadFile(global); strings.Contains(string(cfg), "authama") {
		t.Errorf("the block outlived the identity:\n%s", cfg)
	}
}

func TestGitSignRefusesWhatGitCannotUse(t *testing.T) {
	editHome(t)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	run(t, "ssh", "profile", "add", "work")
	if _, err := runErr(t, "ssh", "git", "sign", "set", "work", "--email", "not an address", "-y"); err == nil {
		t.Error("a bad email was accepted")
	}
	if _, err := runErr(t, "ssh", "git", "sign", "set", "nobody", "--email", "a@x.example", "-y"); err == nil {
		t.Error("an unknown profile was accepted")
	}
	if _, err := runErr(t, "ssh", "git", "sign", "use", "work", t.TempDir()); err == nil {
		t.Error("use without an identity was accepted")
	}
}

func TestGitCloneDryRunNamesTheProfilesURL(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "github-work", "-H", "github.com", "-u", "git")
	out := run(t, "ssh", "git", "clone", "acme/app", "--dry-run")
	if !strings.Contains(out, "clone git@github-work:acme/app.git into app (profile work, profile mode)") {
		t.Errorf("clone --dry-run:\n%s", out)
	}
}
