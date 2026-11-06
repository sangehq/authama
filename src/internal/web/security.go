package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SessionCookie is the cookie a valid token is exchanged for.
const SessionCookie = "authama_session"

// csrfField is the form field, and csrfHeader the request header, that carry
// the per-session CSRF token on every state-changing request.
const (
	csrfField  = "csrf"
	csrfHeader = "X-CSRF-Token"
)

// maxSessions bounds the session table. A session is only created by
// presenting the launch token, so this is a ceiling on memory, not a defence.
const maxSessions = 64

// ContentSecurityPolicy is sent on every response. Everything is same-origin;
// nothing is inline. img-src allows data: because Tabler's CSS draws its
// checkboxes and selects with data: SVG URLs.
const ContentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"connect-src 'self'; font-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

// NewToken returns 32 random bytes, base64url-encoded.
func NewToken() string {
	b := make([]byte, 32)
	// Since Go 1.24 crypto/rand.Read never returns an error: it ends the
	// program itself if the system's generator fails, rather than hand back a
	// predictable token.
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

type session struct {
	csrf    string
	created time.Time

	// pinMu guards the PIN times: requests in one session run concurrently.
	pinMu sync.Mutex
	// unlocked is when the session gave the PIN after signing in, and
	// stepped when it last gave it (at sign-in or on a confirmation).
	unlocked, stepped time.Time
}

func (s *session) unlock(now time.Time) {
	s.pinMu.Lock()
	defer s.pinMu.Unlock()
	if s.unlocked.IsZero() {
		s.unlocked = now
	}
	s.stepped = now
}

func (s *session) unlockedAt() time.Time {
	s.pinMu.Lock()
	defer s.pinMu.Unlock()
	return s.unlocked
}

func (s *session) steppedUpAt() time.Time {
	s.pinMu.Lock()
	defer s.pinMu.Unlock()
	return s.stepped
}

type sessions struct {
	mu   sync.Mutex
	byID map[string]*session
}

func (s *sessions) create(now time.Time) (id string, sess *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = map[string]*session{}
	}
	if len(s.byID) >= maxSessions {
		var oldest string
		for k, v := range s.byID {
			if oldest == "" || v.created.Before(s.byID[oldest].created) {
				oldest = k
			}
		}
		delete(s.byID, oldest)
	}
	id = NewToken()
	sess = &session{csrf: NewToken(), created: now}
	s.byID[id] = sess
	return id, sess
}

func (s *sessions) get(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byID[id]
}

func equal(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// securityHeaders sets the headers every response carries, error pages
// included, so they are set before anything else can answer.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", ContentSecurityPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		// same-origin, not no-referrer: under no-referrer a browser sends
		// "Origin: null" on this server's own form posts, which sameOrigin
		// refuses, so no change could be made from Chromium. same-origin still
		// sends nothing to another site, and the token is gone from the address
		// before any page can link out.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		// The UI uses no device API; a page that tried would be refused.
		h.Set("Permissions-Policy", PermissionsPolicy)
		// No form here is near this size (host specs, the largest, are held
		// to 16 KiB); a bigger body is refused before it is parsed, rather
		// than read into memory up to the form parser's 10 MB default.
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// PermissionsPolicy turns off every device API for these pages.
const PermissionsPolicy = "camera=(), microphone=(), geolocation=(), usb=(), serial=(), bluetooth=(), payment=(), clipboard-read=(), display-capture=()"

// MaxBodyBytes is the largest request body the server reads.
const MaxBodyBytes = 1 << 20

// hostCheck refuses any Host header outside the allowed set.
func (s *Server) hostCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			s.renderError(w, r, http.StatusMisdirectedRequest, "Unknown host",
				"This server only answers to the address it printed when it started.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) hostAllowed(host string) bool {
	for _, h := range s.opts.AllowedHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	return false
}

// auth admits a request carrying a valid session cookie, exchanges a valid
// ?token= for one, and refuses everything else. Static assets are exempt:
// they are the vendored public Tabler files and the 401 page uses them.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			next.ServeHTTP(w, r)
			return
		}
		// /healthz answers without a session: `authama serve` asks it whether
		// the server its record names is the one still listening. It says only
		// that this is authama and which instance - a random id, no state.
		if r.URL.Path == "/healthz" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = fmt.Fprintf(w, `{"app":"authama","instance":%q}`+"\n", s.opts.Instance)
			return
		}
		tok := r.URL.Query().Get("token")
		// A browser already signed in is not signed out by a stray ?token= -
		// a link carrying an old sign-in token, or another page's own
		// parameter by that name: only a matching token is exchanged.
		if tok != "" && !equal(tok, s.opts.Token) && s.signedIn(r) {
			tok = ""
		}
		if tok != "" {
			if !equal(tok, s.opts.Token) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
				s.renderUnauthorized(w, r)
				return
			}
			id, _ := s.sessions.create(s.now())
			http.SetCookie(w, &http.Cookie{
				Name: SessionCookie, Value: id, Path: s.opts.BasePath + "/",
				HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(r),
			})
			// Strip the token from the address bar and the history.
			clean := *r.URL
			q := clean.Query()
			q.Del("token")
			clean.RawQuery = q.Encode()
			target := clean.EscapedPath()
			if clean.RawQuery != "" {
				target += "?" + clean.RawQuery
			}
			if !isLocalURL(clean.Path) {
				target = "/"
			}
			// The same rule as isLocalURL, checked inline on the value that is
			// redirected to, so the check guards the http.Redirect call itself.
			if localPath.MatchString(target) {
				http.Redirect(w, r, target, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/", http.StatusSeeOther)
			}
			return
		}
		c, err := r.Cookie(SessionCookie)
		if err != nil {
			s.renderUnauthorized(w, r)
			return
		}
		sess := s.sessions.get(c.Value)
		if sess == nil {
			s.renderUnauthorized(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(withSession(r.Context(), sess)))
	})
}

// signedIn reports whether r carries a live session cookie.
func (s *Server) signedIn(r *http.Request) bool {
	c, err := r.Cookie(SessionCookie)
	return err == nil && s.sessions.get(c.Value) != nil
}

func (s *Server) secure(r *http.Request) bool {
	return r.TLS != nil || (s.opts.AllowRemote && r.Header.Get("X-Forwarded-Proto") == "https")
}

// sameOrigin enforces, for every request that can change state, that it came
// from this server's own pages: an Origin matching an allowed host (or, when
// a browser omits Origin, Sec-Fetch-Site: same-origin), and the session's
// CSRF token. SameSite=Strict already stops most cross-site requests; this is
// the second and third lock on the same door.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !s.originAllowed(r) {
			s.renderError(w, r, http.StatusForbidden, "Cross-origin request refused",
				"Changes can only be made from this server's own pages.")
			return
		}
		// Parse here, so a body over MaxBodyBytes is reported as what it is
		// rather than as a missing CSRF token.
		if err := r.ParseForm(); err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				s.renderError(w, r, http.StatusRequestEntityTooLarge, "Too large",
					"That form is larger than any change here sends.")
				return
			}
		}
		sess := sessionFrom(r.Context())
		token := r.Header.Get(csrfHeader)
		if token == "" {
			token = r.PostFormValue(csrfField)
		}
		if sess == nil || !equal(token, sess.csrf) {
			s.renderError(w, r, http.StatusForbidden, "Missing or stale form token",
				"Reload the page and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return origin == "" && r.Header.Get("Sec-Fetch-Site") == "same-origin"
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return s.hostAllowed(u.Host) && strings.EqualFold(u.Host, r.Host)
}

// isLocalURL reports whether target is a path on this server, which is the
// only place the sign-in redirect may go. A path starting "//" or "/\\" is
// read by a browser as another host, so /?token= on such a path would send
// the freshly signed-in browser away.
// localPath is isLocalURL as a pattern: "/" alone, or "/" followed by anything
// but a second "/" or a backslash.
var localPath = regexp.MustCompile(`^/(?:$|[^/\\])`)

func isLocalURL(target string) bool {
	return strings.HasPrefix(target, "/") && !strings.HasPrefix(target, "//") && !strings.HasPrefix(target, "/\\")
}
