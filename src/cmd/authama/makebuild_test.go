package main

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// makeDryRun is what `make -n <target>` would run, from the repository top.
func makeDryRun(t *testing.T, target string, vars ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the Makefile's install/link targets are POSIX; Windows uses install")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("no make here")
	}
	cmd := exec.Command("make", append([]string{"-n", target}, vars...)...)
	cmd.Dir = repoTop(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make -n %s: %v\n%s", target, err, out)
	}
	return string(out)
}

// `make build` never leaves build/<os>/authama missing: with ~/.local/bin
// pointing at it (make link) and web.auto_restart watching it, a build that
// emptied build/ first left `authama` off PATH for the whole compile. It
// compiles to a temporary name beside the binary and renames it into place,
// and it does not run clean.
func TestMakeBuildReplacesTheBinaryInOneStep(t *testing.T) {
	out := makeDryRun(t, "build")
	if strings.Contains(out, "find build -mindepth 1") {
		t.Errorf("make build still empties build/ first:\n%s", out)
	}
	var compile, rename string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, " build ") && strings.Contains(line, " -o ") {
			compile = line
		}
		if strings.HasPrefix(strings.TrimSpace(line), "mv -f ") && strings.Contains(line, "/authama") {
			rename = line
		}
	}
	if compile == "" {
		t.Fatalf("no go build line in make -n build:\n%s", out)
	}
	if !strings.Contains(compile, ".tmp") {
		t.Errorf("go build writes the binary in place, not to a temporary name: %s", compile)
	}
	if rename == "" {
		t.Errorf("nothing renames the new binary into place:\n%s", out)
	}
}

// `make link` points BINDIR/authama at this checkout's build, so PATH follows
// every `make build`, and says whether BINDIR is on PATH, as install does.
func TestMakeLinkPointsBinDirAtTheBuild(t *testing.T) {
	out := makeDryRun(t, "link", "BINDIR=/nonexistent/bin")
	if !strings.Contains(out, "ln -sfn") || !strings.Contains(out, "/nonexistent/bin/authama") {
		t.Errorf("make link does not symlink BINDIR/authama:\n%s", out)
	}
	if !strings.Contains(out, "build/") {
		t.Errorf("make link does not point into build/:\n%s", out)
	}
	if !strings.Contains(out, "not on PATH") {
		t.Errorf("make link does not say whether BINDIR is on PATH:\n%s", out)
	}
}
