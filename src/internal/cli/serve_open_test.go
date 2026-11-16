package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/web"
)

// The sign-in link works once, so whoever reads it first holds the session.
// Another local user can read any process's command line (`ps -eo args`), so
// the link must never be on one: the browser is handed an owner-only file that
// redirects to it (audit S-01).
func TestTheBrowserIsNeverGivenTheSignInTokenOnItsCommandLine(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	token := web.NewToken()
	link := "http://127.0.0.1:8384/?token=" + token

	var argv []string
	prev := browserStart
	browserStart = func(a []string) error { argv = append([]string(nil), a...); return nil }
	t.Cleanup(func() { browserStart = prev })

	cleanup, err := openBrowser(dir, link)
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) < 2 {
		t.Fatalf("no browser was started: %q", argv)
	}
	for _, a := range argv {
		if strings.Contains(a, token) {
			t.Fatalf("the token is on the opener's command line: %q", argv)
		}
	}
	page := argv[len(argv)-1]
	if filepath.Dir(page) != dir {
		t.Fatalf("the browser was given %q, not a file in %s", page, dir)
	}
	b, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), token) || !strings.Contains(string(b), "location.replace") ||
		!strings.Contains(string(b), `http-equiv="refresh"`) {
		t.Fatalf("the page does not redirect to the link:\n%s", b)
	}
	// Windows has no mode bits: its permissions are ACLs (util/perms).
	if fi, _ := os.Stat(page); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("the page is %v, not owner-only", fi.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(page); !os.IsNotExist(err) {
		t.Errorf("the page outlived its cleanup: %v", err)
	}
}

// A page left by a launch that could not clean up (a reused server's `serve`
// exits at once) is swept by the next launch once its link has expired.
func TestAStaleSignInPageIsSweptByTheNextLaunch(t *testing.T) {
	dir := t.TempDir()
	prev := browserStart
	browserStart = func([]string) error { return nil }
	t.Cleanup(func() { browserStart = prev })

	if _, err := openBrowser(dir, "http://127.0.0.1:1/?token="+web.NewToken()); err != nil {
		t.Fatal(err)
	}
	old, _ := filepath.Glob(filepath.Join(dir, launchPagePattern))
	if len(old) != 1 {
		t.Fatalf("want one page, have %q", old)
	}
	past := time.Now().Add(-2 * web.LinkLifetime)
	if err := os.Chtimes(old[0], past, past); err != nil {
		t.Fatal(err)
	}
	cleanup, err := openBrowser(dir, "http://127.0.0.1:1/?token="+web.NewToken())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(old[0]); !os.IsNotExist(err) {
		t.Errorf("the expired page %s was not swept", old[0])
	}
}

func TestEachPlatformOpensThePageByItsPath(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux", "freebsd"} {
		argv := browserArgv(goos, "/s/page.html")
		if argv[len(argv)-1] != "/s/page.html" {
			t.Errorf("%s: %q does not end with the page", goos, argv)
		}
	}
}
