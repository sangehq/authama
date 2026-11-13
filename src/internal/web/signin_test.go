package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// clockServer is a test server whose clock the test moves.
func clockServer(t *testing.T) (*Server, *time.Time) {
	t.Helper()
	p, _ := fixtureHome(t)
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, Control: testControl,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Version: "test",
		Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return s, &now
}

// testControl is the control key the link tests present; generated per run.
var testControl = NewToken()

func linkGet(s *Server, tok string) int {
	return do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+"/?token="+tok, nil)).Code
}

// A sign-in link works once, and only within LinkLifetime; a HEAD (a link
// preview) never spends it.
func TestASignInLinkWorksOnceAndExpires(t *testing.T) {
	s, now := clockServer(t)
	if code := do(s, httptest.NewRequest(http.MethodHead, "http://"+testHost+"/?token="+testToken, nil)).Code; code != http.StatusUnauthorized {
		t.Errorf("a HEAD with the link: %d", code)
	}
	if code := linkGet(s, testToken); code != http.StatusSeeOther {
		t.Fatalf("the printed link, first use: %d", code)
	}
	if code := linkGet(s, testToken); code != http.StatusUnauthorized {
		t.Errorf("the printed link, second use: %d", code)
	}
	late := NewLink(s)
	*now = now.Add(LinkLifetime)
	if code := linkGet(s, late.Token); code != http.StatusUnauthorized {
		t.Errorf("a link used %v after it was made: %d", LinkLifetime, code)
	}
	if code := linkGet(s, NewLink(s).Token); code != http.StatusSeeOther {
		t.Errorf("a new link: %d", code)
	}
}

// A session ends after SessionIdle without a request, and SessionMax after
// it began however busy it is.
func TestASessionEndsWhenIdleAndAtItsLimit(t *testing.T) {
	s, now := clockServer(t)
	c := signIn(t, s)
	*now = now.Add(SessionIdle - time.Minute)
	if code := c.get("/keys").Code; code != http.StatusOK {
		t.Fatalf("used just inside the idle limit: %d", code)
	}
	*now = now.Add(SessionIdle)
	if code := c.get("/keys").Code; code != http.StatusUnauthorized {
		t.Errorf("idle for %v: %d", SessionIdle, code)
	}

	c = signIn(t, s)
	for elapsed := time.Duration(0); elapsed < SessionMax-time.Hour; elapsed += time.Hour {
		*now = now.Add(time.Hour)
		if code := c.get("/keys").Code; code != http.StatusOK {
			t.Fatalf("a busy session %v in: %d", elapsed+time.Hour, code)
		}
	}
	*now = now.Add(time.Hour)
	if code := c.get("/keys").Code; code != http.StatusUnauthorized {
		t.Errorf("a busy session at %v: %d", SessionMax, code)
	}
	if body := c.get("/keys").Body.String(); !strings.Contains(body, "works once") {
		t.Error("the sign-in page does not say a link works once")
	}
}

// Only a caller on this machine with the control key gets a new link.
func TestANewLinkNeedsTheControlKeyFromThisMachine(t *testing.T) {
	s, _ := clockServer(t)
	ask := func(key, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://"+testHost+LinkPath, nil)
		r.RemoteAddr = remote
		if key != "" {
			r.Header.Set(ControlHeader, key)
		}
		return do(s, r)
	}
	for _, tc := range []struct{ key, remote string }{
		{"", "127.0.0.1:5000"},
		{NewToken(), "127.0.0.1:5000"},
		{testControl, "192.0.2.10:5000"},
	} {
		if rec := ask(tc.key, tc.remote); rec.Code != http.StatusForbidden {
			t.Errorf("key %v from %s: %d", tc.key != "", tc.remote, rec.Code)
		}
	}
	rec := ask(testControl, "127.0.0.1:5000")
	var reply linkReply
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &reply) != nil || !strings.HasPrefix(reply.SignIn, "?token=") {
		t.Fatalf("with the key: %d %s", rec.Code, rec.Body.String())
	}
	if code := linkGet(s, strings.TrimPrefix(reply.SignIn, "?token=")); code != http.StatusSeeOther {
		t.Errorf("the new link: %d", code)
	}
	// A server with no control key makes no links this way.
	s.opts.Control = ""
	if rec := ask("", "127.0.0.1:5000"); rec.Code != http.StatusForbidden {
		t.Errorf("no control key: %d", rec.Code)
	}
}

// A restart hands over the links not yet used and the sessions not yet
// ended; ended ones and expired links are left behind.
func TestARestartKeepsLiveLinksAndSessionsOnly(t *testing.T) {
	s, now := clockServer(t)
	live := NewLink(s)
	c := signIn(t, s)
	next, err := New(Options{Engine: s.opts.Engine, Token: NewToken(), AllowedHosts: s.opts.AllowedHosts, Version: "test",
		Now: s.opts.Now, Links: PendingLinks(s), Sessions: s.Sessions()})
	if err != nil {
		t.Fatal(err)
	}
	if code := (&client{s: next, cookie: c.cookie}).get("/keys").Code; code != http.StatusOK {
		t.Errorf("the session after a restart: %d", code)
	}
	if code := linkGet(next, live.Token); code != http.StatusSeeOther {
		t.Errorf("a link given out before the restart: %d", code)
	}
	*now = now.Add(SessionIdle)
	later, err := New(Options{Engine: s.opts.Engine, Token: NewToken(), AllowedHosts: s.opts.AllowedHosts, Version: "test",
		Now: s.opts.Now, Links: []LinkState{{Token: NewToken(), Expires: now.Add(-time.Second)}}, Sessions: next.Sessions()})
	if err != nil {
		t.Fatal(err)
	}
	if code := (&client{s: later, cookie: c.cookie}).get("/keys").Code; code != http.StatusUnauthorized {
		t.Errorf("an idle session handed over: %d", code)
	}
	if n := len(PendingLinks(later)); n != 1 { // only its own printed link
		t.Errorf("links after handing over an expired one: %d", n)
	}
}

// A signed-in browser opening a link (the page reloading after a restart)
// keeps its session; the link is spent and leaves the address.
func TestASignedInBrowserSpendsALinkWithoutANewSession(t *testing.T) {
	s, _ := clockServer(t)
	c := signIn(t, s)
	tok := NewLink(s).Token
	rec := c.get("/keys?token=" + tok)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/keys" || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a signed-in browser with a link: %d to %q, cookies %v", rec.Code, rec.Header().Get("Location"), rec.Result().Cookies())
	}
	if code := linkGet(s, tok); code != http.StatusUnauthorized {
		t.Errorf("the link after a signed-in browser used it: %d", code)
	}
}
