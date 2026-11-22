package gitsign

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// sandbox is a dev-root layout: its global git config is <root>/home/.gitconfig.
func sandbox(t *testing.T) paths.Paths {
	t.Helper()
	// Resolved: git matches gitdir: against the real path, and macOS's
	// temporary directory is behind /var -> /private/var.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{SSHDir: filepath.Join(root, "ssh"), ConfigDir: filepath.Join(root, "config"),
		StateRoot: filepath.Join(root, "config"), DevRoot: root}
	for _, d := range []string{p.SSHDir, p.ConfigDir, filepath.Join(root, "home")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// withSigner makes profile work with a declared key "signer" and a git block.
func withSigner(t *testing.T, p paths.Paths, g manifest.ProfileGit, mint bool) *manifest.Manifest {
	t.Helper()
	if g.SigningKey == "" {
		g.SigningKey = "signer"
	}
	m := manifest.Starter(false)
	for i, d := range g.Dirs {
		g.Dirs[i] = filepath.ToSlash(d) // as the CLI and the web UI write them
	}
	m.SetProfile("work", manifest.Profile{KeyScope: "per_service", Keys: []manifest.KeySpec{{Name: "signer"}}, Git: &g})
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(p.Manifest()); err != nil {
		t.Fatal(err)
	}
	if mint {
		key := m.KeyPath(p.SSHDir, manifest.KeyRef{Profile: "work", KeyName: "signer"})
		if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := exec.LookPath("ssh-keygen"); err != nil {
			t.Skip("ssh-keygen not installed")
		}
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "test", "-f", key).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v %s", err, out)
		}
	}
	return m
}

func TestApplyWritesTheFragmentSignersAndOneManagedBlock(t *testing.T) {
	p := sandbox(t)
	m := withSigner(t, p, manifest.ProfileGit{Email: "ada@acme.example", Name: "Ada Lovelace", Sign: true, Dirs: []string{"~/code/acme"}}, true)
	global := GlobalConfigPath(p, func(string) string { return "" })
	if want := filepath.Join(p.DevRoot, "home", ".gitconfig"); global != want {
		t.Fatalf("a sandbox's global config is %s, want %s", global, want)
	}
	mine := "[user]\n\tname = Someone\n[alias]\n\tco = checkout\n"
	if err := os.WriteFile(global, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	pl, err := Render(p, m, nil)
	if err != nil || len(pl.Problems) > 0 {
		t.Fatalf("render: %v %v", err, pl.Problems)
	}
	if pl.UpToDate() {
		t.Fatal("a first apply has something to write")
	}
	res, err := Apply(p, pl, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup == "" {
		t.Fatal("the global config was changed with no copy kept")
	}
	frag, _ := os.ReadFile(FragmentPath(p, "work"))
	for _, want := range []string{"email = ada@acme.example", `name = "Ada Lovelace"`, "signingkey = ", ".pub", "format = ssh", "allowedSignersFile = ", "gpgsign = true"} {
		if !strings.Contains(string(frag), want) {
			t.Errorf("fragment lacks %q:\n%s", want, frag)
		}
	}
	if fi, _ := os.Stat(FragmentPath(p, "work")); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("fragment mode %v, want 0600", fi.Mode().Perm())
	}
	signers, _ := os.ReadFile(AllowedSignersPath(p))
	if !strings.Contains(string(signers), `ada@acme.example namespaces="git" ssh-ed25519 `) || strings.Contains(string(signers), "test\n") {
		t.Errorf("allowed_signers: %s", signers)
	}
	cfg, _ := os.ReadFile(global)
	if !strings.HasPrefix(string(cfg), mine) {
		t.Errorf("the user's own config changed:\n%s", cfg)
	}
	if !strings.Contains(string(cfg), `[includeIf "gitdir:~/code/acme/"]`) || strings.Count(string(cfg), BeginMarker) != 1 {
		t.Errorf("no single managed block:\n%s", cfg)
	}
	again, _ := Render(p, m, nil)
	if !again.UpToDate() {
		t.Errorf("a second apply would change %v", again.Pending())
	}

	// Clearing the profile's identity takes its block and its fragment away, and nothing else.
	m.SetProfile("work", manifest.Profile{KeyScope: "per_service", Keys: []manifest.KeySpec{{Name: "signer"}}})
	pl, _ = Render(p, m, nil)
	if _, err := Apply(p, pl, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	cfg, _ = os.ReadFile(global)
	if string(cfg) != mine+"\n" && string(cfg) != mine {
		t.Errorf("after clearing, the config is not the user's own again:\n%q", cfg)
	}
	if _, err := os.Stat(FragmentPath(p, "work")); !os.IsNotExist(err) {
		t.Error("a cleared profile's fragment stayed")
	}
}

func TestASymlinkedGlobalConfigKeepsItsLink(t *testing.T) {
	p := sandbox(t)
	m := withSigner(t, p, manifest.ProfileGit{Email: "ada@acme.example", Dirs: []string{"/srv/acme"}}, false)
	dotfiles := filepath.Join(p.DevRoot, "dotfiles-gitconfig")
	if err := os.WriteFile(dotfiles, []byte("[core]\n\teditor = vi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(p.DevRoot, "home", ".gitconfig")
	if err := os.Symlink(dotfiles, global); err != nil {
		t.Skip("no symlinks here")
	}
	pl, _ := Render(p, m, nil)
	if _, err := Apply(p, pl, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(global); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the link was replaced by a file")
	}
	b, _ := os.ReadFile(dotfiles)
	if !strings.Contains(string(b), "gitdir:/srv/acme/") || !strings.Contains(string(b), "editor = vi") {
		t.Errorf("the link's target: %s", b)
	}
}

func TestAnUnmadeKeyLeavesSigningOffAndSaysSo(t *testing.T) {
	p := sandbox(t)
	m := withSigner(t, p, manifest.ProfileGit{Email: "ada@acme.example", Sign: true}, false)
	pl, _ := Render(p, m, nil)
	if len(pl.Problems) != 1 || !strings.Contains(pl.Problems[0], "reconcile") {
		t.Fatalf("problems: %v", pl.Problems)
	}
	frag := Fragment(pl.Signers[0], AllowedSignersPath(p))
	if strings.Contains(frag, "gpgsign") || strings.Contains(frag, "signingkey") {
		t.Errorf("signing turned on with no key:\n%s", frag)
	}
}

func TestReplaceBlockKeepsEverythingOutsideTheMarkers(t *testing.T) {
	before := "[a]\n\tx = 1\n"
	after := "[b]\n\ty = 2\n"
	cfg := before + BeginMarker + "\nold\n" + EndMarker + "\n" + after
	got := ReplaceBlock(cfg, BeginMarker+"\nnew\n"+EndMarker+"\n")
	if got != before+BeginMarker+"\nnew\n"+EndMarker+"\n"+after {
		t.Errorf("got %q", got)
	}
	if got := ReplaceBlock(cfg, ""); got != before+after {
		t.Errorf("removing: %q", got)
	}
	if got := ReplaceBlock("", ""); got != "" {
		t.Errorf("nothing to do wrote %q", got)
	}
}

// TestACheckoutUnderTheDirectoryCommitsAndSignsAsTheProfile is the whole path:
// apply, a checkout under the profile's directory, a signed commit, verify.
func TestACheckoutUnderTheDirectoryCommitsAndSignsAsTheProfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	p := sandbox(t)
	code := filepath.Join(p.DevRoot, "code", "acme")
	m := withSigner(t, p, manifest.ProfileGit{Email: "ada@acme.example", Name: "Ada", Sign: true, Dirs: []string{code}}, true)
	pl, _ := Render(p, m, nil)
	if _, err := Apply(p, pl, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(p.DevRoot, "home")
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("SSH_AUTH_SOCK", "")
	repo := filepath.Join(code, "app")
	run := func(dir string, args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	run(repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(repo, "add", "README")
	run(repo, "commit", "-q", "-m", "first")
	st := Status(repo)
	if st.Email != "ada@acme.example" || !st.Sign || st.Format != "ssh" || !strings.HasSuffix(st.From, "work.gitconfig") {
		t.Fatalf("status: %+v", st)
	}
	commits, err := Verify(repo, AllowedSignersPath(p), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 1 || !commits[0].Good() || commits[0].Signer != "ada@acme.example" {
		t.Fatalf("verify: %+v", commits)
	}

	// A checkout elsewhere is made the profile's with `use`, and switching
	// takes the other authama fragment out but keeps the user's own include.
	other := filepath.Join(p.DevRoot, "elsewhere")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	run(other, "init", "-q")
	run(other, "config", "--local", "--add", "include.path", "/etc/mine.gitconfig")
	run(other, "config", "--local", "--add", "include.path", filepath.Join(Dir(p), "old.gitconfig"))
	res, err := Use(other, FragmentPath(p, "work"), Dir(p), false)
	if err != nil || !res.Added || len(res.Replaced) != 1 {
		t.Fatalf("use: %+v %v", res, err)
	}
	inc := run(other, "config", "--local", "--get-all", "include.path")
	if !strings.Contains(inc, "/etc/mine.gitconfig") || strings.Contains(inc, "old.gitconfig") || !strings.Contains(inc, "work.gitconfig") {
		t.Errorf("includes after use: %s", inc)
	}
}

// A value that could read as an option, or holds a control character, never
// reaches git; a directory is where git runs, not an argument.
func TestGitRefusesAValueThatCouldBeAnOption(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"-c", "--upload-pack=x", "a\nb", "", "x\x00"} {
		if _, err := git(dir, nil, []string{"config", "--get"}, bad); err == nil || !strings.Contains(err.Error(), "cannot be given to git") {
			t.Errorf("git accepted %q: %v", bad, err)
		}
	}
	if _, err := git(filepath.Join(dir, "missing"), nil, []string{"rev-parse"}); err == nil {
		t.Error("git ran in a directory that does not exist")
	}
}

func TestCheckGitVersionNamesTheMinimum(t *testing.T) {
	for out, ok := range map[string]bool{
		"git version 2.34.0": true, "git version 2.56.1": true, "git version 3.0.0": true,
		"git version 2.39.5 (Apple Git-154)": true, "git version 2.33.8": false, "git version 2.30.2": false,
		"something else": true,
	} {
		err := checkGitVersion(out)
		if (err == nil) != ok {
			t.Errorf("checkGitVersion(%q) = %v", out, err)
		}
		if err != nil && !strings.Contains(err.Error(), MinGit) {
			t.Errorf("the error does not name git %s: %v", MinGit, err)
		}
	}
}
