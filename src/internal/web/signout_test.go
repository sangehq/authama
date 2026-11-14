package web

import (
	"net/http"
	"strings"
	"testing"
)

// Signing out ends the session on the server, not only in the browser: the
// cookie is cleared, and the same cookie sent again is refused (R-GAP-05).
func TestSignOutEndsTheSession(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	sess := s.sessions.get(c.cookie.Value, s.now())

	if rec := c.get("/"); rec.Code != http.StatusOK {
		t.Fatalf("signed in: GET / = %d", rec.Code)
	}
	if !strings.Contains(c.get("/").Body.String(), `action="/signout"`) {
		t.Error("the user menu offers no sign-out")
	}

	rec := c.post("/signout", csrfField+"="+sess.csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /signout = %d, want 200", rec.Code)
	}
	cleared := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == SessionCookie && ck.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("sign-out did not clear the session cookie")
	}
	if !strings.Contains(rec.Body.String(), "Signed out") {
		t.Error("sign-out page does not say the browser is signed out")
	}
	if rec := c.get("/"); rec.Code != http.StatusUnauthorized {
		t.Errorf("the old cookie still works after sign-out: GET / = %d", rec.Code)
	}
}

// Sign-out is a change like any other: without the session's form token it
// is refused, so another site cannot sign a browser out.
func TestSignOutNeedsTheFormToken(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	if rec := c.post("/signout", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("POST /signout without a token = %d, want 403", rec.Code)
	}
	if rec := c.get("/"); rec.Code != http.StatusOK {
		t.Errorf("a refused sign-out ended the session: GET / = %d", rec.Code)
	}
}
