package cli

import (
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/version"
	"github.com/sangehq/authama/src/internal/web"
)

func TestBuildsAreOrderedByReleaseThenCommits(t *testing.T) {
	for _, c := range []struct {
		a, b string
		cmp  int
		ok   bool
	}{
		{"v0.1.0-53-g9ee110d", "v0.1.0-52-g0c4cfd3", 1, true},
		{"v0.1.0", "v0.1.0-3-gabc1234", -1, true},
		{"v0.2.0", "v0.1.9-99-gabc1234", 1, true},
		{"v0.1.0-5-gabc1234-dirty", "v0.1.0-5-gdef5678", 0, true},
		{"dev", "v0.1.0", 0, false},
		{"v0.1.0", "", 0, false},
	} {
		cmp, ok := compareVersions(c.a, c.b)
		if cmp != c.cmp || ok != c.ok {
			t.Errorf("compareVersions(%q, %q) = %d, %v; want %d, %v", c.a, c.b, cmp, ok, c.cmp, c.ok)
		}
	}
}

func TestABrowserIsOpenedOnlyWhereThereIsADesktop(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	for _, c := range []struct {
		goos string
		get  func(string) string
		ok   bool
	}{
		{"darwin", env(), true},
		{"windows", env(), true},
		{"linux", env("DISPLAY", ":0"), true},
		{"linux", env("WAYLAND_DISPLAY", "wayland-0"), true},
		{"linux", env(), false},
		{"freebsd", env(), false},
		{"darwin", env("SSH_CONNECTION", "10.0.0.2 51000 10.0.0.1 22"), false},
		{"linux", env("DISPLAY", ":0", "SSH_TTY", "/dev/pts/1"), false},
	} {
		if ok, why := canOpenBrowser(c.goos, c.get); ok != c.ok || (!ok && why == "") {
			t.Errorf("%s: canOpenBrowser = %v (%q), want %v", c.goos, ok, why, c.ok)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener here:", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestALaunchReplacesAServerThatIsOutOfDate(t *testing.T) {
	rec := &serveRecord{Addr: "127.0.0.1:7822", Version: "v0.1.0-52-g0c4cfd3"}
	if fix := replaceReason(rec, "v0.1.0-53-g9ee110d", "", 0, "", ""); fix.Port != 7822 || !strings.Contains(fix.Why, "older build") {
		t.Errorf("an older build: %+v", fix)
	}
	if fix := replaceReason(rec, "v0.1.0-50-gaaaaaaa", "", 0, "", ""); fix.Why != "" {
		t.Errorf("a newer build is running, and was to be replaced by an older one: %+v", fix)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if fix := replaceReason(rec, "dev", exe, 0, "", exe); !strings.Contains(fix.Why, "rebuilt") {
		t.Errorf("the same binary, rebuilt: %+v", fix)
	}
	same := &serveRecord{Addr: "127.0.0.1:7822", Version: "dev"}
	if fix := replaceReason(same, "dev", "", 0, "/authama", ""); !strings.Contains(fix.Why, "web.base_path") {
		t.Errorf("another base path: %+v", fix)
	}
	free := freePort(t)
	if fix := replaceReason(same, "dev", "", free, "", ""); fix.Port != free || !strings.Contains(fix.Why, "web.port is "+strconv.Itoa(free)) {
		t.Errorf("off web.port, which is free: %+v", fix)
	}
	if fix := replaceReason(same, "dev", "", 7822, "", ""); fix.Why != "" {
		t.Errorf("a server on web.port: %+v", fix)
	}
}

// A launch with an older build running stops it and starts this build on
// the same port; a newer build running is opened as it is.
func TestALaunchUpgradesTheRunningServer(t *testing.T) {
	p := editHome(t)
	prevV := version.Version
	t.Cleanup(func() { version.Version = prevV })
	version.Version = "v0.1.0-53-g9ee110d"
	rec := serveRecord{PID: 4242, Addr: "127.0.0.1:7840", Instance: "old", Version: "v0.1.0-40-gabcdef0", Started: time.Now()}
	if err := writeServeRecord(p, rec); err != nil {
		t.Fatal(err)
	}
	prevAlive, prevLink := serveAlive, serveNewLink
	t.Cleanup(func() { serveAlive, serveNewLink = prevAlive, prevLink })
	serveAlive = func(r *serveRecord) bool { return r.Instance == "old" }
	serveNewLink = func(*serveRecord) (string, error) { return "http://127.0.0.1:7840/?token=" + web.NewToken(), nil }

	tried, stop := fakeServe(t)
	var stopped []string
	serveStop = func(r *serveRecord) error { stopped = append(stopped, r.Instance); return nil }
	out, err := runServe(t, stop)
	if err != nil || len(stopped) != 1 || len(*tried) == 0 || (*tried)[0] != "127.0.0.1:7840" ||
		!strings.Contains(out, "replacing the web UI at 127.0.0.1:7840 (pid 4242): it runs v0.1.0-40-gabcdef0, an older build") {
		t.Fatalf("older build: %v, stopped %v, bound %v\n%s", err, stopped, *tried, out)
	}

	rec.Version = "v0.2.0"
	if err := writeServeRecord(p, rec); err != nil {
		t.Fatal(err)
	}
	tried, stop = fakeServe(t)
	stopped = nil
	serveStop = func(r *serveRecord) error { stopped = append(stopped, r.Instance); return nil }
	out, err = runServe(t, stop)
	if err != nil || len(stopped) != 0 || len(*tried) != 0 || !strings.Contains(out, "a newer build (v0.2.0) is running") {
		t.Fatalf("newer build: %v, stopped %v, bound %v\n%s", err, stopped, *tried, out)
	}
}

// A record whose server holds its port but does not answer is reaped, its
// record removed, and a working server started.
func TestALaunchClearsAHungServer(t *testing.T) {
	p := editHome(t)
	if err := writeServeRecord(p, serveRecord{PID: 4242, Addr: "127.0.0.1:7822", Instance: "hung", Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	prevAlive := serveAlive
	t.Cleanup(func() { serveAlive = prevAlive })
	serveAlive = func(*serveRecord) bool { return false }
	tried, stop := fakeServe(t)
	reaped := 0
	serveHung = func(r *serveRecord) bool { return r.Instance == "hung" }
	serveReap = func(*serveRecord) error { reaped++; return nil }
	out, err := runServe(t, stop)
	if err != nil || reaped != 1 || len(*tried) == 0 || !strings.Contains(out, "it held its port but no longer answered") {
		t.Fatalf("hung server: %v, reaped %d, bound %v\n%s", err, reaped, *tried, out)
	}
}

// runServeOpen is runServe without --no-open.
func runServeOpen(t *testing.T, stop context.CancelFunc, args ...string) string {
	t.Helper()
	root := newRootCmd()
	var out syncBuffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"serve"}, args...))
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	for deadline := time.After(10 * time.Second); ; {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			return out.String()
		case <-deadline:
			stop()
			t.Fatalf("serve neither served nor exited:\n%s", out.String())
		case <-time.After(10 * time.Millisecond):
			if strings.Contains(out.String(), "press Ctrl-C to stop") {
				stop()
				if err := <-done; err != nil {
					t.Fatalf("serve after stop: %v\n%s", err, out.String())
				}
				return out.String()
			}
		}
	}
}

// Every launch on a desktop opens the browser with a fresh link - the running
// server's too - and none does over SSH or with --no-open.
func TestEveryLaunchOpensTheBrowserWithAFreshLink(t *testing.T) {
	p := editHome(t)
	for _, v := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(v, "")
	}
	t.Setenv("DISPLAY", ":0")
	_, stop := fakeServe(t)
	serveInteractive = func() bool { return true }
	var opened []string
	serveBrowser = func(_ string, link string) (func(), error) { opened = append(opened, link); return func() {}, nil }

	// Nothing running: the new server's link.
	if out := runServeOpen(t, stop); len(opened) != 1 || !strings.Contains(opened[0], "?token=") {
		t.Fatalf("a new server opened %q\n%s", opened, out)
	}
	// A server running: a fresh link from it.
	if err := writeServeRecord(p, serveRecord{PID: 4242, Addr: "127.0.0.1:7822", Instance: "abc", Version: version.Version, Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	prevAlive, prevLink := serveAlive, serveNewLink
	t.Cleanup(func() { serveAlive, serveNewLink = prevAlive, prevLink })
	serveAlive = func(r *serveRecord) bool { return r.Instance == "abc" }
	fresh := "http://127.0.0.1:7822/?token=" + web.NewToken()
	serveNewLink = func(*serveRecord) (string, error) { return fresh, nil }
	if out := runServeOpen(t, stop); len(opened) != 2 || opened[1] != fresh || !strings.Contains(out, "opened it in your browser") {
		t.Fatalf("the running server: opened %q\n%s", opened, out)
	}
	// --no-open, and an SSH session, open nothing.
	_ = runServeOpen(t, stop, "--no-open")
	t.Setenv("SSH_CONNECTION", "10.0.0.2 51000 10.0.0.1 22")
	out := runServeOpen(t, stop)
	if len(opened) != 2 || !strings.Contains(out, "not opening a browser: this is an SSH session") {
		t.Fatalf("--no-open or SSH opened %q\n%s", opened, out)
	}
}
