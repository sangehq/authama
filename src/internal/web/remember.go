package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/sangehq/authama/src/internal/services/browsers"
	"github.com/sangehq/authama/src/internal/util/applog"
)

// BrowserCookie remembers a browser between sessions (web.remember_days): a
// browser that signed in with a one-time link is signed in again by it when
// it opens the address with no session, so the address alone works.
//
// It is HttpOnly (no script reads it), SameSite=Strict (no other site's page
// can make the browser send it), scoped to the base path, and Secure behind
// TLS. The server keeps only a hash of its secret, in the home's state
// directory, so it is bound to this home: another home's server, or the same
// home's after the browser is forgotten, does not take it. Each use replaces
// its secret, and an old secret seen again forgets the browser (see package
// browsers). Changes still ask for the owner PIN where they did.
const BrowserCookie = "authama_browser"

// remembered is the remembered-browser half of signing in, apart from
// Server so its method count stays down.
type remembered struct{ s *Server }

// remembering reports whether this server remembers browsers.
func (m remembered) remembering() bool { return m.s.opts.RememberFor > 0 }

func (m remembered) browsersFile() string {
	return browsers.File(m.s.opts.Engine.Home().StateDir())
}

func (m remembered) setBrowserCookie(w http.ResponseWriter, r *http.Request, value string) {
	s := m.s
	http.SetCookie(w, &http.Cookie{
		Name: BrowserCookie, Value: value, Path: s.opts.BasePath + "/",
		MaxAge: int(s.opts.RememberFor / time.Second), Expires: s.now().Add(s.opts.RememberFor),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(r),
	})
}

func (m remembered) clearBrowserCookie(w http.ResponseWriter, r *http.Request) {
	s := m.s
	http.SetCookie(w, &http.Cookie{
		Name: BrowserCookie, Value: "", Path: s.opts.BasePath + "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(r),
	})
}

// rememberBrowser records the browser behind r, which just signed in with a
// link, and gives it its cookie. A cookie it already held names a browser
// that is replaced, so a browser opened by every launch is one entry, not one
// per launch. It returns the browser's id, or "" when nothing is remembered.
func (m remembered) rememberBrowser(w http.ResponseWriter, r *http.Request) string {
	s := m.s
	if !m.remembering() {
		return ""
	}
	if c, err := r.Cookie(BrowserCookie); err == nil {
		if id, _, ok := browsers.ParseCookie(c.Value); ok {
			_, _ = browsers.Forget(m.browsersFile(), []string{id}, false, s.now())
		}
	}
	id, sec, err := browsers.Remember(m.browsersFile(), browsers.Label(r.UserAgent()), s.now(), s.opts.RememberFor)
	if err != nil {
		return ""
	}
	m.setBrowserCookie(w, r, browsers.Cookie(id, sec))
	return id
}

// signInRemembered signs in a browser that has no session but is remembered:
// only for a GET or HEAD, which change nothing, so a forged request from
// elsewhere gains nothing even if a browser sent the cookie with it. The
// cookie's secret is replaced, and a new session begins.
func (m remembered) signInRemembered(w http.ResponseWriter, r *http.Request) *session {
	s := m.s
	if !m.remembering() || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
		return nil
	}
	c, err := r.Cookie(BrowserCookie)
	if err != nil {
		return nil
	}
	id, sec, ok := browsers.ParseCookie(c.Value)
	if !ok {
		m.clearBrowserCookie(w, r)
		return nil
	}
	next, ok, err := browsers.SignIn(m.browsersFile(), id, sec, s.now(), s.opts.RememberFor)
	if err != nil || !ok {
		m.clearBrowserCookie(w, r)
		if errors.Is(err, browsers.ErrReused) {
			logAuth(r, "browser.reused", id)
		}
		return nil
	}
	if next != "" {
		m.setBrowserCookie(w, r, browsers.Cookie(id, next))
	}
	sid, sess := s.sessions.create(s.now())
	sess.browser = id
	m.setSessionCookie(w, r, sid)
	logAuth(r, "signin.browser", id)
	return sess
}

// browserForgotten reports whether sess was signed in by a remembered browser
// that has since been forgotten (on the Settings page, by `authama serve
// browsers forget`, or by signing out): its session ends with it.
func (m remembered) browserForgotten(sess *session) bool {
	s := m.s
	return sess.browser != "" && !browsers.Known(m.browsersFile(), sess.browser, s.now())
}

func (m remembered) setSessionCookie(w http.ResponseWriter, r *http.Request, id string) {
	s := m.s
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: id, Path: s.opts.BasePath + "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(r),
	})
}

// BrowserRow is one remembered browser on the Settings page.
type BrowserRow struct {
	browsers.Browser
	This bool
}

// browserRows is the Settings page's list of remembered browsers, with the
// one the page is shown to marked.
func (m remembered) browserRows(r *http.Request) []BrowserRow {
	s := m.s
	list, err := browsers.List(m.browsersFile(), s.now())
	if err != nil {
		return nil
	}
	this := ""
	if sess := sessionFrom(r.Context()); sess != nil {
		this = sess.browser
	}
	out := make([]BrowserRow, 0, len(list))
	for _, b := range list {
		out = append(out, BrowserRow{Browser: b, This: b.ID == this})
	}
	return out
}

// forgetBrowsers answers POST /browsers/forget: id names one browser, or
// all=1 every one. The browser the request came from, if forgotten, is
// signed out too.
type forgetBrowsers struct{ s *Server }

func (p forgetBrowsers) handle(w http.ResponseWriter, r *http.Request) {
	s := p.s
	m := remembered{s}
	all := r.PostFormValue("all") == "1"
	id := r.PostFormValue("id")
	var ids []string
	if id != "" {
		ids = []string{id}
	}
	_, err := browsers.Forget(m.browsersFile(), ids, all, s.now())
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, "Could not forget", err.Error())
		return
	}
	logAuth(r, "browser.forgotten", id)
	if sess := sessionFrom(r.Context()); sess != nil && sess.browser != "" && (all || sess.browser == id) {
		m.clearBrowserCookie(w, r)
	}
	http.Redirect(w, r, s.opts.BasePath+"/settings#browsers", http.StatusSeeOther)
}

// logAuth records a sign-in event in authama.log: which, from where, and the
// remembered browser's id (never its cookie).
func logAuth(r *http.Request, event, browser string) {
	args := []any{"area", "auth", "remote", r.RemoteAddr, "path", r.URL.Path}
	if browser != "" {
		args = append(args, "browser", browser)
	}
	level := applog.Info
	if event == "browser.reused" || event == "signin.refused" {
		level = applog.Warn
	}
	level(event, args...)
}
