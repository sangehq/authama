package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// The version an installer installs - an argument, an environment variable
// or an answer from GitHub's API - becomes part of the download URL. Anything
// but vX.Y.Z (with an optional pre-release) is refused before a byte is
// fetched, so a value like ../.. or a query string cannot move the download
// (audit A-SEC-07, 2026-11-16).
func TestTheInstallersTakeOnlyAReleaseVersion(t *testing.T) {
	scripts := filepath.Join(repoRoot(t), "scripts")
	// The PowerShell installer is checked by its source: no pwsh is assumed.
	ps, err := os.ReadFile(filepath.Join(scripts, "install.ps1")) //nolint:gosec // the repository's own installer
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ps), installerVersionPattern) {
		t.Errorf("install.ps1 does not check the version against %s", installerVersionPattern)
	}
	sh, err := os.ReadFile(filepath.Join(scripts, "install.sh")) //nolint:gosec // the repository's own installer
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sh), installerVersionPattern) {
		t.Errorf("install.sh does not check the version against %s", installerVersionPattern)
	}
	re := regexp.MustCompile(installerVersionPattern)
	for v, ok := range map[string]bool{"v0.1.0": true, "v1.20.3-rc.1": true, "v0.1": false, "../../x": false, "v0.1.0/../../x": false, "v0.1.0?x=1": false} {
		if re.MatchString(v) != ok {
			t.Errorf("the pattern says %s is a release version: %v", v, !ok)
		}
	}

	if runtime.GOOS == "windows" {
		return
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	empty := t.TempDir()
	for _, v := range []string{"../../../x", "v0.1.0/../../x", "v0.1.0?x=1"} {
		cmd := exec.Command(bash, filepath.Join(scripts, "install.sh"), v) //nolint:gosec // the repository's own installer
		cmd.Env = append(os.Environ(), "AUTHAMA_RELEASE_BASE=file://"+empty, "AUTHAMA_INSTALL_DIR="+t.TempDir(), "HOME="+t.TempDir(), "TMPDIR="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "not a release version") {
			t.Errorf("install.sh %q: %v\n%s", v, err, out)
		}
	}
}

// installerVersionPattern is the check both installers carry, verbatim.
const installerVersionPattern = `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`
