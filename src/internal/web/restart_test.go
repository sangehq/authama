package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// "Restart now" answers with the old instance and how to sign back in, then
// calls Restart; a server that cannot restart says so instead.
func TestAServerRestartsItselfFromTheBanner(t *testing.T) {
	s, _, _ := newTestServer(t)
	called := make(chan struct{}, 1)
	s.opts.Restart = func() error { called <- struct{}{}; return nil }
	c := signIn(t, s)
	rec := c.change("/server/restart", url.Values{})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("restart: %d %s", rec.Code, rec.Body.String())
	}
	var reply restartReply
	// The reply carries a new link, not the used one, and it is among the
	// links handed to the next server.
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || reply.Instance != s.opts.Instance {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	tok := strings.TrimPrefix(reply.SignIn, "?token=")
	handed := false
	for _, l := range PendingLinks(s) {
		handed = handed || l.Token == tok
	}
	if tok == "" || tok == s.opts.Token || !handed {
		t.Fatalf("reply = %+v: not a new link handed to the next server", reply)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("Restart was not called")
	}

	// Without a CSRF token, nothing restarts.
	if rec := c.post("/server/restart", ""); rec.Code == http.StatusAccepted {
		t.Error("a restart without a CSRF token was accepted")
	}

	s.opts.Restart = nil
	if rec := c.change("/server/restart", url.Values{}); rec.Code != http.StatusNotImplemented ||
		!strings.Contains(rec.Body.String(), "cannot restart itself") {
		t.Errorf("no Restart: %d %s", rec.Code, rec.Body.String())
	}
}

// About marks the running build current or older, and every page's banner
// offers a restart when it is older; the header carries no build strip any
// more. /healthz names its process only to this machine.
func TestThePageSaysWhetherItsBuildIsCurrent(t *testing.T) {
	s, _, _ := newTestServer(t)
	s.opts.Restart = func() error { return nil }
	c := signIn(t, s)
	exe := filepath.Join(t.TempDir(), "authama")
	if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	s.binary = binaryWatch{path: exe, started: time.Now().Add(time.Hour)}
	body := c.get("/hosts").Body.String()
	if strings.Contains(body, "test · current") || strings.Contains(body, "data-restart-server") {
		t.Error("a page other than About carries the build mark, or a restart for a current build")
	}
	if !strings.Contains(body, "data-restarted hidden") {
		t.Error("the restarted confirmation is missing")
	}
	if !strings.Contains(body, "data-running-as") {
		t.Error("the account menu does not say who the server runs as")
	}
	if about := c.get("/about").Body.String(); !strings.Contains(about, "test · current") {
		t.Error("About does not mark an unchanged binary current")
	}

	s.binary = binaryWatch{path: exe, started: time.Now().Add(-time.Hour)}
	body = c.get("/hosts").Body.String()
	if !strings.Contains(body, "data-restart-server") || strings.Contains(body, "data-restarted hidden") {
		t.Error("a rebuilt binary's page offers no restart")
	}
	if about := c.get("/about").Body.String(); !strings.Contains(about, "test · older build") {
		t.Error("About does not mark a rebuilt binary older")
	}

	local := httptestGet("/healthz")
	local.RemoteAddr = "127.0.0.1:50000"
	var h struct {
		App, Instance, Version string
		PID                    int
	}
	if err := json.Unmarshal(do(s, local).Body.Bytes(), &h); err != nil || h.PID != os.Getpid() || h.Version != "test" {
		t.Errorf("local healthz = %+v, %v", h, err)
	}
	remote := httptestGet("/healthz")
	remote.RemoteAddr = "192.0.2.10:50000"
	if body := do(s, remote).Body.String(); strings.Contains(body, `"pid"`) || strings.Contains(body, strconv.Itoa(os.Getpid())) {
		t.Errorf("healthz told another machine its pid: %s", body)
	}
}

// A page signed in before a restart is still signed in after it: the new
// server adopts the sessions the old one hands over.
func TestSessionsSurviveARestart(t *testing.T) {
	old, p, _ := newTestServer(t)
	c := signIn(t, old)
	next, err := New(Options{Engine: AppEngine{P: p}, Token: NewToken(), AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil),
		Version: "test", Sessions: old.Sessions()})
	if err != nil {
		t.Fatal(err)
	}
	moved := &client{s: next, cookie: c.cookie}
	if rec := moved.get("/hosts"); rec.Code != http.StatusOK {
		t.Fatalf("after the handoff: %d", rec.Code)
	}
	// The CSRF token moved too, so a form from before the restart still posts.
	if old.sessions.get(c.cookie.Value, old.now()).csrf != next.sessions.get(c.cookie.Value, next.now()).csrf {
		t.Error("the CSRF token changed across the restart")
	}
	if body := moved.get("/hosts").Body.String(); !strings.Contains(body, `name="authama-instance"`) || !strings.Contains(body, "data-live-build") {
		t.Error("the page cannot follow a restart: no instance meta or live-build element")
	}
}

// The watcher restarts once the rebuilt binary holds still, never while it
// is still changing, never from a removed one, and not at all when off.
func TestARebuiltBinaryRestartsTheServerOnceItSettles(t *testing.T) {
	s, _, _ := newTestServer(t)
	exe := filepath.Join(t.TempDir(), "authama")
	if err := os.WriteFile(exe, []byte("new build"), 0o755); err != nil { //nolint:gosec // a test executable
		t.Fatal(err)
	}
	restarts := make(chan struct{}, 4)
	s.opts.Restart = func() error { restarts <- struct{}{}; return nil }
	s.binary = binaryWatch{path: exe, started: time.Now().Add(-time.Hour)}

	// Each watcher has stopped before the options change under it: they are
	// read without a lock, as a running server never changes them.
	off, offDone := make(chan struct{}), make(chan struct{})
	s.opts.AutoRestart = false
	go func() { s.watchBinary(off, 5*time.Millisecond); close(offDone) }()
	select {
	case <-restarts:
		t.Fatal("restarted with auto_restart off")
	case <-time.After(60 * time.Millisecond):
	}
	close(off)
	<-offDone

	stop := make(chan struct{})
	s.opts.AutoRestart = true
	go s.watchBinary(stop, 5*time.Millisecond)
	select {
	case <-restarts:
	case <-time.After(2 * time.Second):
		t.Fatal("a settled rebuilt binary did not restart the server")
	}
	close(stop)

	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	stop2 := make(chan struct{})
	defer close(stop2)
	go s.watchBinary(stop2, 5*time.Millisecond)
	select {
	case <-restarts:
		t.Fatal("restarted from a removed binary")
	case <-time.After(60 * time.Millisecond):
	}
}

// A server whose binary moved and that finds no other to restart from says
// why, instead of answering 202 and leaving the page waiting for a server
// that never starts (found 2026-11-13, after the checkout moved to sangehq).
func TestARestartWithNoBinaryToRunSaysWhy(t *testing.T) {
	s, _, _ := newTestServer(t)
	called := make(chan struct{}, 1)
	s.opts.Restart = func() error { called <- struct{}{}; return nil }
	s.opts.RestartFrom = func() (string, error) {
		return "", errors.New("the binary this server started from is gone, and no authama is on PATH")
	}
	c := signIn(t, s)
	rec := c.change("/server/restart", url.Values{})
	if rec.Code == http.StatusAccepted || !strings.Contains(rec.Body.String(), "no authama is on PATH") {
		t.Fatalf("restart with nothing to run: %d %s", rec.Code, rec.Body.String())
	}
	select {
	case <-called:
		t.Fatal("Restart was called with nothing to run")
	case <-time.After(300 * time.Millisecond):
	}
}

// The watcher follows a binary that moved: with the path it started from
// gone, it watches the one RestartFrom names, and restarts once that one is
// rebuilt and holds still.
func TestTheWatcherFollowsAMovedBinary(t *testing.T) {
	s, _, _ := newTestServer(t)
	dir := t.TempDir()
	moved := filepath.Join(dir, "authama")
	if err := os.WriteFile(moved, []byte("new build"), 0o755); err != nil { //nolint:gosec // a test executable
		t.Fatal(err)
	}
	restarts := make(chan struct{}, 1)
	s.opts.Restart = func() error { restarts <- struct{}{}; return nil }
	s.opts.AutoRestart = true
	s.opts.RestartFrom = func() (string, error) { return moved, nil }
	s.binary = binaryWatch{path: filepath.Join(dir, "gone", "authama"), started: time.Now().Add(-time.Hour)}
	stop := make(chan struct{})
	defer close(stop)
	go s.watchBinary(stop, 5*time.Millisecond)
	select {
	case <-restarts:
	case <-time.After(2 * time.Second):
		t.Fatal("a rebuilt binary at its new place did not restart the server")
	}
}
