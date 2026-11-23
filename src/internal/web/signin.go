package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
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
	// seen are the link challenges answered, by when each stops being
	// accepted, so a recorded request cannot be replayed.
	seen map[string]time.Time
}

// fresh reports whether challenge has not been answered before, and marks it
// answered until until. Expired marks are dropped as it goes.
func (l *links) fresh(challenge string, now, until time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]time.Time{}
	}
	for c, t := range l.seen {
		if now.After(t) {
			delete(l.seen, c)
		}
	}
	if _, ok := l.seen[challenge]; ok || len(l.seen) >= 4*maxLinks {
		return false
	}
	l.seen[challenge] = until
	return true
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
	// Proof is LinkProof, when the request carried a challenge.
	Proof string `json:"proof,omitempty"`
}

// LinkChallengeHeader and LinkProofHeader ask for a new sign-in link in one
// request without sending the control key: the caller's challenge
// ("<unix seconds>.<random>") and LinkCallerProof over it. The answer carries
// LinkProof, so the caller trusts the link only from the server that holds
// the key. Before, the caller proved the server at /healthz and then sent the
// key itself on a second connection, which a process taking the port between
// the two received (audit P-02).
const (
	LinkChallengeHeader = "X-Authama-Challenge"
	LinkProofHeader     = "X-Authama-Proof"
)

// linkSkew is how far a link challenge's time may be from the server's.
const linkSkew = 2 * time.Minute

// LinkChallenge is a fresh challenge for a link request made at now.
func LinkChallenge(now time.Time) string {
	return strconv.FormatInt(now.Unix(), 10) + "." + NewToken()
}

// LinkCallerProof proves the caller holds the control key: an HMAC of the
// server's instance and the challenge, under a label of its own.
func LinkCallerProof(control, instance, challenge string) string {
	m := hmac.New(sha256.New, []byte(control))
	_, _ = m.Write([]byte("authama-link-caller\x00" + instance + "\x00" + challenge))
	return hex.EncodeToString(m.Sum(nil))
}

// LinkProof proves the server that answered holds the control key, bound to
// the caller's challenge and the link it gives.
func LinkProof(control, challenge, signIn string) string {
	m := hmac.New(sha256.New, []byte(control))
	_, _ = m.Write([]byte("authama-link\x00" + challenge + "\x00" + signIn))
	return hex.EncodeToString(m.Sum(nil))
}

// handleLink answers `authama serve` with a new sign-in link. Only a caller on
// this machine that holds the control key gets one: the key is in the
// server's record in the state home (mode 0600), beside nothing a browser can
// read. The caller proves it with LinkCallerProof over a fresh, timely
// challenge, and the answer proves the server with LinkProof; the key itself
// crosses no connection. The key in ControlHeader, as builds before
// 2026-11-22 send it, is still taken. A server started without a key makes no
// links this way.
func handleLink(s *Server, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.opts.Control == "" || !isLoopbackRequest(r) || forwarded(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	challenge := r.Header.Get(LinkChallengeHeader)
	switch {
	case challenge != "":
		now := s.now()
		secs, _, ok := strings.Cut(challenge, ".")
		at, err := strconv.ParseInt(secs, 10, 64)
		if !ok || err != nil || len(challenge) > maxChallenge ||
			now.Sub(time.Unix(at, 0)).Abs() > linkSkew ||
			!hmac.Equal([]byte(r.Header.Get(LinkProofHeader)), []byte(LinkCallerProof(s.opts.Control, s.opts.Instance, challenge))) ||
			!s.links.fresh(challenge, now, now.Add(2*linkSkew)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	case !equal(r.Header.Get(ControlHeader), s.opts.Control):
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	st := NewLink(s)
	reply := linkReply{SignIn: "?token=" + st.Token, Expires: st.Expires}
	if challenge != "" {
		reply.Proof = LinkProof(s.opts.Control, challenge, reply.SignIn)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(reply)
}

// expired reports whether a session last seen at seen, begun at created, has
// ended by now.
func expired(created, seen, now time.Time) bool {
	return now.Sub(created) >= SessionMax || now.Sub(seen) >= SessionIdle
}
