package web

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// A sign-in link works once, and only for LinkLifetime after it was made: the
// address `authama serve` prints, or one it asks the running server for later.
// A session ends after SessionIdle without a request, or SessionMax after it
// began, whichever comes first. The owner chose these on 2026-11-13 ("review
// and fix"); before, a link signed in any number of browsers for as long as
// the server ran, and a session never ended.
const (
	LinkLifetime = 10 * time.Minute
	SessionIdle  = 8 * time.Hour
	SessionMax   = 24 * time.Hour
)

// maxLinks bounds the links waiting to be used. Making one takes the control
// key, so this is a ceiling on memory, not a defence.
const maxLinks = 32

// ControlHeader carries the control key on a request for a new sign-in link.
const ControlHeader = "X-Authama-Control"

// LinkPath is where `authama serve` asks a running server for a new link.
const LinkPath = "/auth/link"

// LinkState is a sign-in link not yet used, handed to the server replacing
// this one so a link already given out still works after a restart.
type LinkState struct {
	Token   string    `json:"token"`
	Expires time.Time `json:"expires"`
}

type links struct {
	mu    sync.Mutex
	byTok map[string]time.Time
}

func (l *links) add(tok string, expires time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byTok == nil {
		l.byTok = map[string]time.Time{}
	}
	if len(l.byTok) >= maxLinks {
		var oldest string
		for k, v := range l.byTok {
			if oldest == "" || v.Before(l.byTok[oldest]) {
				oldest = k
			}
		}
		delete(l.byTok, oldest)
	}
	l.byTok[tok] = expires
}

// issue makes a new link that works once until LinkLifetime from now.
func (l *links) issue(now time.Time) LinkState {
	st := LinkState{Token: NewToken(), Expires: now.Add(LinkLifetime)}
	l.add(st.Token, st.Expires)
	return st
}

// take uses tok up: it reports whether tok was a link still waiting, and
// forgets it either way. Every waiting link is compared in constant time.
func (l *links) take(tok string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	var found string
	for k := range l.byTok {
		if equal(k, tok) {
			found = k
		}
	}
	if found == "" {
		return false
	}
	expires := l.byTok[found]
	delete(l.byTok, found)
	return now.Before(expires)
}

// pending is every link still waiting and not yet expired.
func (l *links) pending(now time.Time) []LinkState {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LinkState, 0, len(l.byTok))
	for k, v := range l.byTok {
		if now.Before(v) {
			out = append(out, LinkState{Token: k, Expires: v})
		}
	}
	return out
}

// PendingLinks is every sign-in link s gave out and nobody used yet, for the
// server replacing it. A function, not a method: Server is held to 24.
func PendingLinks(s *Server) []LinkState { return s.links.pending(s.now()) }

// NewLink makes a sign-in link on s that works once, for LinkLifetime.
func NewLink(s *Server) LinkState { return s.links.issue(s.now()) }

// linkReply is what LinkPath answers.
type linkReply struct {
	SignIn  string    `json:"sign_in"` // "?token=...", to add to the server's address
	Expires time.Time `json:"expires"`
}

// handleLink answers `authama serve` with a new sign-in link. Only a caller on
// this machine that presents the control key gets one: the key is in the
// server's record in the state home (mode 0600), beside nothing a browser can
// read. A server started without a key makes no links this way.
func handleLink(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Control == "" || !isLoopbackRequest(r) || !equal(r.Header.Get(ControlHeader), s.opts.Control) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	st := NewLink(s)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(linkReply{SignIn: "?token=" + st.Token, Expires: st.Expires})
}

// expired reports whether a session last seen at seen, begun at created, has
// ended by now.
func expired(created, seen, now time.Time) bool {
	return now.Sub(created) >= SessionMax || now.Sub(seen) >= SessionIdle
}
