//go:build e2e && !windows

package main

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTokenEndToEnd drives the token group through both shipped binaries,
// authama and the sshmgr compatibility name, in a sandbox.
//
// Every run passes --dev-root, HOME is a fresh temp dir, and the environment
// is built from scratch: only PATH comes from the caller, so a developer's
// own GH_TOKEN, COMPOSER_HOME, DOCKER_CONFIG, NPM_CONFIG_USERCONFIG or NETRC
// can neither feed the run nor redirect its writes. The last subtest proves
// nothing was written outside the sandbox.
func TestTokenEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	dir := t.TempDir()
	bins := map[string]string{}
	for name, pkg := range map[string]string{"authama": "../authama", "sshmgr": "."} {
		bins[name] = filepath.Join(dir, name)
		if out, err := exec.Command("go", "build", "-o", bins[name], pkg).CombinedOutput(); err != nil {
			t.Fatalf("building %s: %v\n%s", name, err, out)
		}
	}
	home := filepath.Join(t.TempDir(), "home")
	root := filepath.Join(t.TempDir(), "sandbox")
	for _, d := range []string{home, root} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USERPROFILE=" + home,
		"SSH_MANAGER_HOME=" + filepath.Join(home, "cfg"),
		"SSH_MANAGER_AUTO_PIN=0",
		"GH_TOKEN=SECRET_E2E_GH",
		"NPM_TOKEN=SECRET_E2E_NPM",
		// Set on purpose: dev mode must ignore them.
		"COMPOSER_HOME=" + filepath.Join(home, "leak-composer"),
		"NPM_CONFIG_USERCONFIG=" + filepath.Join(home, "leak.npmrc"),
		"DOCKER_CONFIG=" + filepath.Join(home, "leak-docker"),
		"NETRC=" + filepath.Join(home, "leak.netrc"),
	}
	run := func(bin, stdin string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(bins[bin], append([]string{"--dev-root", root}, args...)...)
		cmd.Env, cmd.Stdin = env, strings.NewReader(stdin)
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return string(out), ee.ExitCode()
		} else if err != nil {
			t.Fatalf("running %v: %v", args, err)
		}
		return string(out), 0
	}
	ok := func(bin, stdin string, args ...string) string {
		t.Helper()
		out, code := run(bin, stdin, args...)
		if code != 0 {
			t.Fatalf("%s %v exited %d:\n%s", bin, args, code, out)
		}
		return out
	}
	cfg := filepath.Join(root, "tokens.json")
	if err := os.WriteFile(cfg, []byte(`{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"}},`+
		`"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("sync writes the configured targets and prints no secret", func(t *testing.T) {
		out := ok("authama", "", "token", "sync", "--config", cfg, "--no-env-hint")
		if strings.Contains(out, "SECRET_E2E") {
			t.Errorf("sync printed a secret:\n%s", out)
		}
		if !strings.Contains(out, "wrote composer") || !strings.Contains(out, "wrote npm") {
			t.Errorf("sync:\n%s", out)
		}
	})

	t.Run("store reads the secret from stdin", func(t *testing.T) {
		ok("authama", "SECRET_E2E_GIT\n", "token", "store", "--target", "git", "--server", "github.com", "--username", "octocat")
	})

	t.Run("inventory lists every entry and masks the secrets", func(t *testing.T) {
		out := ok("authama", "", "token", "inventory")
		for _, want := range []string{"composer", "npm", "git", "github.com", "registry.npmjs.org"} {
			if !strings.Contains(out, want) {
				t.Errorf("inventory lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "SECRET_E2E") {
			t.Errorf("inventory printed a secret:\n%s", out)
		}
	})

	t.Run("the sshmgr compatibility binary reaches the same token group", func(t *testing.T) {
		a := ok("authama", "", "token", "inventory", "--json")
		s := ok("sshmgr", "", "token", "inventory", "--json")
		if a != s {
			t.Errorf("authama and sshmgr disagree:\n%s\n---\n%s", a, s)
		}
	})

	t.Run("checkup finds the files written with private modes", func(t *testing.T) {
		ok("authama", "", "token", "checkup")
	})

	t.Run("forget removes one entry and leaves the rest", func(t *testing.T) {
		if before := ok("authama", "", "token", "inventory", "--json"); !strings.Contains(before, "octocat") {
			t.Fatalf("the git entry is not listed before forget:\n%s", before)
		}
		ok("authama", "", "token", "forget", "git", "github.com")
		out := ok("authama", "", "token", "inventory", "--json")
		if strings.Contains(out, "octocat") {
			t.Errorf("git entry survived forget:\n%s", out)
		}
		if !strings.Contains(out, "registry.npmjs.org") {
			t.Errorf("forget removed too much:\n%s", out)
		}
	})

	t.Run("a near-miss verb is refused, not prefix-matched", func(t *testing.T) {
		if out, code := run("authama", "", "token", "inv"); code == 0 {
			t.Errorf("`token inv` ran:\n%s", out)
		}
	})

	t.Run("nothing was written outside the sandbox", func(t *testing.T) {
		_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(home, p)
			if strings.HasPrefix(rel, "cfg"+string(filepath.Separator)) {
				return nil // the tool's own state dir
			}
			t.Errorf("a token run wrote %s outside --dev-root", p)
			return nil
		})
		for _, rel := range []string{".npmrc", ".git-credentials"} {
			fi, err := os.Stat(filepath.Join(root, "home", rel))
			if err == nil && fi.Mode().Perm() != 0o600 {
				t.Errorf("%s is %04o, want 0600", rel, fi.Mode().Perm())
			}
		}
	})
}
