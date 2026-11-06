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
	if !strings.Contains(body, "test, current") || strings.Contains(body, "data-restart-server") {
		t.Error("an unchanged binary is not marked current")
	}
	if !strings.Contains(body, "data-restarted hidden") {
		t.Error("the restarted confirmation is missing")
	}

	s.binary = binaryWatch{path: exe, started: time.Now().Add(-time.Hour)}
	body = c.get("/hosts").Body.String()
	if !strings.Contains(body, "test, older build") || !strings.Contains(body, "data-restart-server") ||
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
