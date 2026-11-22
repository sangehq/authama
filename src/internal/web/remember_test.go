package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/browsers"
)

// newRememberingServer is a test server that remembers browsers for a day.
func newRememberingServer(t *testing.T) *Server {
	t.Helper()
	p, _ := fixtureHome(t)
	s, err := New(Options{
		Engine:       AppEngine{P: p},
		Token:        testToken,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil),
		Version:      "test",
		RememberFor:  24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// linkSignIn signs in with a fresh link and returns both cookies.
func linkSignIn(t *testing.T, s *Server) (sess, browser *http.Cookie) {
	t.Helper()
	rec := do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+"/?token="+NewLink(s).Token, nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("link sign-in: %d", rec.Code)
	}
	return cookieNamed(rec, SessionCookie), cookieNamed(rec, BrowserCookie)
}

func getWith(s *Server, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://"+testHost+path, nil)
	for _, c := range cookies {
		if c != nil {
			r.AddCookie(c)
		}
	}
	return do(s, r)
}

// A sign-in link leaves a browser cookie that is HttpOnly, SameSite=Strict
// and scoped to the server's path; the next visit without a session (a
// restart, a closed browser) signs in with it, and swaps its secret.
func TestARememberedBrowserSignsStraightBackIn(t *testing.T) {
	s := newRememberingServer(t)
	_, bc := linkSignIn(t, s)
	if bc == nil {
		t.Fatal("no browser cookie after a link sign-in")
	}
	if !bc.HttpOnly || bc.SameSite != http.SameSiteStrictMode || bc.Path != "/" || bc.MaxAge <= 0 {
		t.Fatalf("browser cookie attributes: %+v", bc)
	}
	rec := getWith(s, "/", bc)
	if rec.Code != http.StatusOK {
		t.Fatalf("a remembered browser got %d", rec.Code)
	}
	next := cookieNamed(rec, BrowserCookie)
	if next == nil || next.Value == bc.Value || cookieNamed(rec, SessionCookie) == nil {
		t.Fatal("signing in with the browser cookie did not rotate it and start a session")
	}
	// The rotated cookie works on a later visit.
	if rec := getWith(s, "/", next); rec.Code != http.StatusOK {
		t.Fatalf("rotated cookie: %d", rec.Code)
	}
}

// Only a GET or HEAD signs in with the browser cookie: a POST that carries
// it and nothing else (a forged form, whatever the browser sends) is refused.
func TestTheBrowserCookieAloneMakesNoChange(t *testing.T) {
	s := newRememberingServer(t)
	_, bc := linkSignIn(t, s)
	r := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/signout", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://"+testHost)
	r.AddCookie(bc)
	if rec := do(s, r); rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST with only the browser cookie = %d, want 401", rec.Code)
	}
}

// The cookie is checked against the server's Host rules first: a rebinding
// name never reaches sign-in.
func TestTheBrowserCookieIsRefusedOnAForeignHost(t *testing.T) {
	s := newRememberingServer(t)
	_, bc := linkSignIn(t, s)
	r := httptest.NewRequest(http.MethodGet, "http://evil.example:7822/", nil)
	r.AddCookie(bc)
	if rec := do(s, r); rec.Code == http.StatusOK {
		t.Fatal("a foreign Host signed in with the browser cookie")
	}
}

// A copied cookie used after the browser already swapped it forgets the
// browser for good, and both copies stop working.
func TestAReplayedBrowserCookieForgetsTheBrowser(t *testing.T) {
	s := newRememberingServer(t)
	_, bc := linkSignIn(t, s)
	rec := getWith(s, "/", bc)
	next := cookieNamed(rec, BrowserCookie)
	s.opts.Now = func() time.Time { return time.Now().Add(2 * browsers.Grace) }
	if rec := getWith(s, "/", bc); rec.Code != http.StatusUnauthorized {
		t.Fatalf("replayed old cookie = %d", rec.Code)
	}
	if rec := getWith(s, "/", next); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the newer cookie still works after a replay: %d", rec.Code)
	}
}

// Forgetting a browser on the Settings page ends its session at once.
func TestForgettingABrowserSignsItOut(t *testing.T) {
	s := newRememberingServer(t)
	sc, bc := linkSignIn(t, s)
	page := getWith(s, "/settings", sc).Body.String()
	if !strings.Contains(page, "Remembered browsers") || !strings.Contains(page, "this browser") {
		t.Fatal("Settings does not list this browser")
	}
	sess := s.sessions.get(sc.Value, s.now())
	r := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/browsers/forget", strings.NewReader("all=1&"+csrfField+"="+sess.csrf))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://"+testHost)
	r.AddCookie(sc)
	if rec := do(s, r); rec.Code != http.StatusSeeOther {
		t.Fatalf("forget all = %d", rec.Code)
	}
	if rec := getWith(s, "/", sc, bc); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after forget, the session still works: %d", rec.Code)
	}
}

// Signing out forgets the browser too, or its cookie would sign it back in.
func TestSigningOutForgetsTheBrowser(t *testing.T) {
	s := newRememberingServer(t)
	sc, bc := linkSignIn(t, s)
	sess := s.sessions.get(sc.Value, s.now())
	r := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/signout", strings.NewReader(csrfField+"="+sess.csrf))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://"+testHost)
	r.AddCookie(sc)
	r.AddCookie(bc)
	rec := do(s, r)
	if c := cookieNamed(rec, BrowserCookie); c == nil || c.MaxAge >= 0 {
		t.Fatal("sign-out left the browser cookie")
	}
	if rec := getWith(s, "/", bc); rec.Code != http.StatusUnauthorized {
		t.Fatalf("the browser cookie signs in after sign-out: %d", rec.Code)
	}
}

// With remembering off (web.remember_days 0) no browser cookie is set.
func TestRememberingCanBeTurnedOff(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+"/?token="+NewLink(s).Token, nil))
	if cookieNamed(rec, BrowserCookie) != nil {
		t.Fatal("a browser cookie with remembering off")
	}
}

// The 401 page tells you how to get in: open authama. It offers no button
// that signs a browser in, and the token in a URL is never echoed.
func TestTheUnauthorizedPageSaysOpenAuthama(t *testing.T) {
	s, _, _ := newTestServer(t)
	rec := do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+"/keys?token=wrong", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusUnauthorized || !strings.Contains(body, "Open authama to sign in") {
		t.Fatalf("401 page: %d\n%s", rec.Code, body)
	}
	if strings.Contains(body, "wrong") {
		t.Fatal("the 401 page echoes the token it was given")
	}
	if strings.Contains(strings.ToLower(body), "<form") {
		t.Fatal("the 401 page has a form: nothing on it may sign a browser in")
	}
}
