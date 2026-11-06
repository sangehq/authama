package web

import (
	"encoding/json"
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
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil || reply.Instance != s.opts.Instance ||
		reply.SignIn != "?token="+s.opts.Token {
		t.Fatalf("reply = %+v, %v", reply, err)
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

// The header marks the running build current, or older with a way to
// restart; /healthz names its process only to this machine.
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
	if !strings.Contains(body, "test · current") || strings.Contains(body, "data-restart-server") {
		t.Error("an unchanged binary is not marked current")
	}
	if !strings.Contains(body, "data-restarted hidden") {
		t.Error("the restarted confirmation is missing")
	}

	s.binary = binaryWatch{path: exe, started: time.Now().Add(-time.Hour)}
	body = c.get("/hosts").Body.String()
	if !strings.Contains(body, "test · older build") || !strings.Contains(body, "data-restart-server") ||
		strings.Contains(body, "data-restarted hidden") {
		t.Error("a rebuilt binary is not marked older with a restart button")
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
	if old.sessions.get(c.cookie.Value).csrf != next.sessions.get(c.cookie.Value).csrf {
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

	stop := make(chan struct{})
	s.opts.AutoRestart = false
	go s.watchBinary(stop, 5*time.Millisecond)
	select {
	case <-restarts:
		t.Fatal("restarted with auto_restart off")
	case <-time.After(60 * time.Millisecond):
	}

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
