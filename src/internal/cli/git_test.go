package cli

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/gitremote"
)

func gitRepo(t *testing.T, origin string) string {
	t.Helper()
	needBin(t, "git")
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "remote", "add", "origin", origin}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func TestGitStatusAndDryRunUse(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "imanimanyara")
	run(t, "ssh", "host", "add", "imanimanyara", "github-imanimanyara", "-H", "github.com", "-u", "git")
	repo := gitRepo(t, "https://github.com/simtabi/pheg.git")

	out := run(t, "ssh", "git", "status", repo)
	if !strings.Contains(out, "https  simtabi/pheg  profile=imanimanyara (by owner)") {
		t.Errorf("status:\n%s", out)
	}
	out = run(t, "ssh", "git", "use", "profile", repo, "--dry-run")
	if !strings.Contains(out, "would change  https://github.com/simtabi/pheg.git -> git@github-imanimanyara:simtabi/pheg.git") {
		t.Errorf("dry run:\n%s", out)
	}
	if got, _ := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output(); strings.TrimSpace(string(got)) != "https://github.com/simtabi/pheg.git" {
		t.Errorf("dry run changed origin to %s", got)
	}
	// ssh mode with no defaults.git.profile has no key bound to Host github.com: refused, not guessed.
	if _, err := runErr(t, "ssh", "git", "use", "ssh", repo, "--dry-run"); err == nil {
		t.Error("ssh mode without defaults.git.profile should fail")
	}
	if out, _ := runErr(t, "ssh", "git", "use", "https", t.TempDir(), "--dry-run"); !strings.Contains(out, "no origin") {
		t.Errorf("a directory with no repository is skipped and said so:\n%s", out)
	}
}

func TestGitModePrecedence(t *testing.T) {
	m := &manifest.Manifest{Defaults: manifest.Defaults{Git: &manifest.GitDefaults{Mode: "https"}}}
	t.Setenv(gitModeEnv, "")
	if got, _ := resolveGitMode("", m); got != "https" {
		t.Errorf("manifest default: %q", got)
	}
	t.Setenv(gitModeEnv, "ssh")
	if got, _ := resolveGitMode("", m); got != "ssh" {
		t.Errorf("env over manifest: %q", got)
	}
	if got, _ := resolveGitMode("profile", m); got != "profile" {
		t.Errorf("explicit over env: %q", got)
	}
	t.Setenv(gitModeEnv, "")
	if got, _ := resolveGitMode("", &manifest.Manifest{}); got != "profile" {
		t.Errorf("nothing set: %q", got)
	}
	if _, err := resolveGitMode("ftp", m); err == nil {
		t.Error("unknown mode accepted")
	}
}

// The text `ssh git status` prints, one line per checkout in each of its
// states, is what people read and scripts already grep: it stays as it is.
func TestGitStatusTextIsUnchanged(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "imanimanyara")
	run(t, "ssh", "host", "add", "imanimanyara", "github-imanimanyara", "-H", "github.com", "-u", "git")
	byOwner := gitRepo(t, "https://github.com/simtabi/pheg.git")
	byRemote := gitRepo(t, "git@github-imanimanyara:simtabi/authama.git")
	other := gitRepo(t, "https://gitlab.com/simtabi/x.git")
	none := t.TempDir()

	out := run(t, "ssh", "git", "status", byOwner, byRemote, other, none)
	want := []string{
		byOwner + "  https  simtabi/pheg  profile=imanimanyara (by owner)",
		byRemote + "  profile  simtabi/authama  profile=imanimanyara",
		other + "  -  not a managed GitHub remote (https://gitlab.com/simtabi/x.git)",
		none + "  -  no origin",
	}
	if got := strings.TrimSpace(out); got != strings.Join(want, "\n") {
		t.Errorf("status:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
}

// `ssh git status --json` is read by other tools (sange's contract tests), so
// its shape is pinned here, field by field.
func TestGitStatusJSON(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "imanimanyara")
	run(t, "ssh", "host", "add", "imanimanyara", "github-imanimanyara", "-H", "github.com", "-u", "git")
	byOwner := gitRepo(t, "https://github.com/simtabi/pheg.git")
	byRemote := gitRepo(t, "git@github-imanimanyara:simtabi/authama.git")
	other := gitRepo(t, "https://gitlab.com/simtabi/x.git")
	none := t.TempDir()

	out := run(t, "ssh", "git", "status", "--json", byOwner, byRemote, other, none)
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	want := map[string]any{
		"version":      1.0,
		"default_mode": "profile",
		"checkouts": []any{
			map[string]any{"path": byOwner, "state": "managed", "url": "https://github.com/simtabi/pheg.git",
				"mode": "https", "owner": "simtabi", "repo": "pheg", "profile": "imanimanyara", "profile_from": "owner"},
			map[string]any{"path": byRemote, "state": "managed", "url": "git@github-imanimanyara:simtabi/authama.git",
				"mode": "profile", "owner": "simtabi", "repo": "authama", "profile": "imanimanyara", "profile_from": "remote"},
			map[string]any{"path": other, "state": "unmanaged", "url": "https://gitlab.com/simtabi/x.git"},
			map[string]any{"path": none, "state": "no-origin"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		g, _ := json.MarshalIndent(got, "", "  ")
		w, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("ssh git status --json:\n%s\nwant:\n%s", g, w)
	}
}

// Fields of `ssh git status --json`, `use --json`, `resolve --json` and
// `companion info --json` are only ever added: each one other
// tools may read is still there under its name. Removing or renaming one
// fails here; bump the document's version instead, and say so in the
// CHANGELOG.
func TestGitStatusJSONKeepsItsFields(t *testing.T) {
	names := func(v any) map[string]bool {
		out := map[string]bool{}
		ty := reflect.TypeOf(v)
		for i := 0; i < ty.NumField(); i++ {
			if tag := strings.Split(ty.Field(i).Tag.Get("json"), ",")[0]; tag != "" {
				out[tag] = true
			}
		}
		return out
	}
	for _, tc := range []struct {
		v    any
		want []string
	}{
		{gitStatusDoc{}, []string{"version", "default_mode", "checkouts"}},
		{gitremote.Checkout{}, []string{"path", "state", "url", "mode", "owner", "repo", "profile", "profile_from"}},
		{gitUseDoc{}, []string{"version", "mode", "dry_run", "checkouts"}},
		{gitUseRow{}, []string{"path", "from", "to", "applied", "skipped", "error", "owner", "repo", "profile", "profile_from"}},
		{gitResolveDoc{}, []string{"version", "profile", "mode", "url", "hostAlias", "keyFingerprint"}},
		{companionInfoDoc{}, []string{"name", "version", "contractVersions", "capabilities", "configHome", "stateHome"}},
	} {
		have := names(tc.v)
		if len(have) < len(tc.want) {
			t.Errorf("%T has %d fields, fewer than the %d it published", tc.v, len(have), len(tc.want))
		}
		for _, n := range tc.want {
			if !have[n] {
				t.Errorf("%T no longer has %q, which other tools read", tc.v, n)
			}
		}
	}
}
