package web

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/sangehq/authama/src/internal/services/owner"
)

// ownerGate is the PIN's hold on the web UI. It is its own type, beside
// Server rather than on it, so Server does not grow into the facade the
// layering test guards against.
type ownerGate struct{ s *Server }

// stepUpWindow is how long a PIN given at sign-in, or on a confirmation,
// covers the changes that follow. After it, the next change asks again.
const stepUpWindow = 5 * time.Minute

// pinField is the form field a confirmation carries the PIN in.
const pinField = "pin"

// pinRequired reports whether the home has a PIN set. It reads owner.json on
// each request, so setting or removing a PIN applies without a restart.
func (g ownerGate) pinRequired() bool {
	s := g.s
	c, err := owner.Load(s.opts.Engine.Home())
	// An owner.json that cannot be read gates everything: failing open would
	// let a damaged file switch the PIN off.
	return err != nil || c.HasPIN()
}

// locked reports whether sess has yet to give the PIN.
func (g ownerGate) locked(sess *session) bool {
	return sess != nil && sess.unlockedAt().IsZero() && g.pinRequired()
}

// needsStepUp reports whether a change from sess must carry the PIN.
func (g ownerGate) needsStepUp(sess *session) bool {
	s := g.s
	if sess == nil || !g.pinRequired() {
		return false
	}
	return s.now().Sub(sess.steppedUpAt()) > stepUpWindow
}

// checkPIN checks a PIN for sess and, if it is right, marks the session
// unlocked and stepped up. Wrong answers count towards the same lockout the
// command line uses.
func (g ownerGate) checkPIN(sess *session, pin string) error {
	s := g.s
	if pin == "" {
		return owner.ErrWrongPIN
	}
	if err := owner.CheckPIN(s.opts.Engine.Home(), pin, s.now()); err != nil {
		return err
	}
	sess.unlock(s.now())
	return nil
}

// stepUp is the gate on a confirmed change: a PIN in the form, or one given
// within stepUpWindow. It renders the refusal itself and reports false.
func (g ownerGate) stepUp(w http.ResponseWriter, r *http.Request) bool {
	s := g.s
	sess := sessionFrom(r.Context())
	if !g.needsStepUp(sess) {
		return true
	}
	err := g.checkPIN(sess, r.PostForm.Get(pinField))
	if err == nil {
		return true
	}
	var locked owner.LockedError
	switch {
	case errors.As(err, &locked):
		s.renderError(w, r, http.StatusTooManyRequests, "Too many wrong PINs", err.Error())
	case errors.Is(err, owner.ErrWrongPIN):
		s.renderError(w, r, http.StatusForbidden, "PIN needed",
			"This change needs your PIN, and the one given was empty or wrong. Go back, enter it on the confirmation, and confirm again.")
	default:
		s.renderError(w, r, http.StatusInternalServerError, "The PIN could not be checked", err.Error())
	}
	return false
}

type unlockPage struct {
	Next    string
	Problem string
}

// handleUnlock asks for the PIN after the sign-in link, before anything else.
func (g ownerGate) handleUnlock(w http.ResponseWriter, r *http.Request) {
	s := g.s
	sess := sessionFrom(r.Context())
	next := r.URL.Query().Get("next")
	if r.Method == http.MethodPost {
		next = r.PostForm.Get("next")
	}
	if !localPath.MatchString(next) || next == "/unlock" {
		next = "/"
	}
	if !g.locked(sess) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	status, problem := http.StatusOK, ""
	if r.Method == http.MethodPost {
		err := g.checkPIN(sess, r.PostForm.Get(pinField))
		if err == nil {
			// The same rule as localPath, checked on the value redirected to.
			if localPath.MatchString(next) {
				http.Redirect(w, r, next, http.StatusSeeOther)
			} else {
				http.Redirect(w, r, "/", http.StatusSeeOther)
			}
			return
		}
		status, problem = http.StatusForbidden, "That PIN is not the one set for this home."
		var locked owner.LockedError
		if errors.As(err, &locked) {
			status, problem = http.StatusTooManyRequests, err.Error()
		} else if !errors.Is(err, owner.ErrWrongPIN) {
			status, problem = http.StatusInternalServerError, err.Error()
		}
	}
	v := s.newView(r, "", "Unlock", unlockPage{Next: next, Problem: problem})
	v.Authed = false // no navigation until the PIN is given
	s.render(w, status, "unlock", v)
}

// requireUnlock sends a session that has not given the PIN to /unlock.
func (g ownerGate) requireUnlock(next http.Handler) http.Handler {
	s := g.s
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if r.URL.Path == "/unlock" || sess == nil || !g.locked(sess) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.renderError(w, r, http.StatusForbidden, "Locked", "Give the PIN first.")
			return
		}
		target := r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, "/unlock?next="+url.QueryEscape(target), http.StatusSeeOther)
	})
}
