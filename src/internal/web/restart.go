package web

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"
)

// restartAPI restarts the server from the binary now installed, so a page
// that says it is from an older build can be brought up to date without a
// terminal. Its own type, so the pages type keeps to its size.
type restartAPI struct{ s *Server }

// restartReply tells the page where to sign in once the new server answers:
// the same address and the same token, since the old process's sessions end
// with it. The page already holds a session, so the token tells it nothing
// it could not already do.
type restartReply struct {
	Instance string `json:"instance"` // the old server's, to wait for /healthz to answer another
	SignIn   string `json:"sign_in"`  // the path and token to sign in with
}

// handleRestart answers, then hands over: Restart starts the new server,
// which waits for this one to stop answering before it listens, and stops
// this one. A server that cannot restart says so.
func (x restartAPI) handleRestart(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if x.s.opts.Restart == nil {
		w.WriteHeader(http.StatusNotImplemented)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "this server cannot restart itself; stop it and run `authama serve` again"})
		return
	}
	w.WriteHeader(http.StatusAccepted)
	// A new link, handed to the next server with the others not yet used, for
	// a page whose session did not survive the restart.
	_ = json.NewEncoder(w).Encode(restartReply{Instance: x.s.opts.Instance, SignIn: "?token=" + NewLink(x.s).Token})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	restart := x.s.opts.Restart
	go func() {
		time.Sleep(200 * time.Millisecond) // let the reply reach the page first
		_ = restart()
	}()
}

// isLoopbackRequest reports whether r came from this machine.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// SessionState is a signed-in session handed to the server that replaces
// this one, so a page signed in before a restart is still signed in after
// it. The PIN unlock is not handed over: owner-only changes ask again.
type SessionState struct {
	ID      string    `json:"id"`
	CSRF    string    `json:"csrf"`
	Created time.Time `json:"created"`
	// Seen is when it last made a request; a server before this field hands
	// over none, and the session counts as seen when it is adopted.
	Seen time.Time `json:"seen,omitempty"`
}

// Sessions is every signed-in session, for the server replacing this one.
func (s *Server) Sessions() []SessionState {
	s.sessions.mu.Lock()
	defer s.sessions.mu.Unlock()
	out := make([]SessionState, 0, len(s.sessions.byID))
	for id, sess := range s.sessions.byID {
		out = append(out, SessionState{ID: id, CSRF: sess.csrf, Created: sess.created, Seen: sess.seen})
	}
	return out
}

// adopt takes over sessions a restarted server handed over.
func (s *sessions) adopt(states []SessionState, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = map[string]*session{}
	}
	for _, st := range states {
		if st.ID != "" && st.CSRF != "" && len(s.byID) < maxSessions {
			seen := st.Seen
			if seen.IsZero() {
				seen = now
			}
			if !expired(st.Created, seen, now) {
				s.byID[st.ID] = &session{csrf: st.CSRF, created: st.Created, seen: seen}
			}
		}
	}
}

// watchBinary restarts the server when its binary is rebuilt or reinstalled:
// once the file at its path has changed and then held still for two checks
// in a row (a build still being written is never launched), Restart runs.
// A removed binary is waited out, never restarted from.
func (s *Server) watchBinary(stop <-chan struct{}, every time.Duration) {
	if s.opts.Restart == nil || !s.opts.AutoRestart || s.binary.path == "" {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	var last os.FileInfo
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if s.binary.replaced() == "" {
			last = nil
			continue
		}
		fi, err := os.Stat(s.binary.path)
		// Windows has no executable bit; elsewhere a file without one is
		// not a finished build.
		if err != nil || (runtime.GOOS != "windows" && fi.Mode()&0o111 == 0) {
			last = nil
			continue
		}
		if last != nil && fi.Size() == last.Size() && fi.ModTime().Equal(last.ModTime()) {
			_ = s.opts.Restart()
			return
		}
		last = fi
	}
}
