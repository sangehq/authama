package web

import "net/http"

// handleSignOut ends the browser's session on the server and clears its
// cookie (R-GAP-05). It is a POST behind the same-origin and form-token
// checks every change passes, so another site cannot sign a browser out.
// Read-only servers offer it too: it changes nothing but the session.
func (s *Server) handleSignOut(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil {
		s.sessions.end(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: s.opts.BasePath + "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(r),
	})
	s.renderError(w, r, http.StatusOK, "Signed out",
		"This browser is signed out, and its session has ended on the server. Run `authama serve` for a new sign-in link.")
}
