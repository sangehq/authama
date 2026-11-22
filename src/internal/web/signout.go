package web

import (
	"net/http"

	"github.com/sangehq/authama/src/internal/services/browsers"
)

// signOut ends the browser's session on the server and clears its
// cookie (R-GAP-05). It is a POST behind the same-origin and form-token
// checks every change passes, so another site cannot sign a browser out.
// Read-only servers offer it too: it changes nothing but the session.
type signOut struct{ s *Server }

// handle answers POST /signout.
func (p signOut) handle(w http.ResponseWriter, r *http.Request) {
	s := p.s
	rb := remembered{s}
	if c, err := r.Cookie(SessionCookie); err == nil {
		s.sessions.end(c.Value)
	}
	// Signing out forgets this browser too: else its cookie would sign it
	// straight back in on the next page.
	if sess := sessionFrom(r.Context()); sess != nil && sess.browser != "" {
		_, _ = browsers.Forget(rb.browsersFile(), []string{sess.browser}, false, s.now())
	}
	rb.clearBrowserCookie(w, r)
	logAuth(r, "signout", "")
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: s.opts.BasePath + "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(r),
	})
	s.renderError(w, r, http.StatusOK, "Signed out",
		"This browser is signed out, and its session has ended on the server. Open authama again (run `authama`, or double-click it) to sign in.")
}
