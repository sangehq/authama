package web

import (
	"encoding/json"
	"net"
	"net/http"
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
	_ = json.NewEncoder(w).Encode(restartReply{Instance: x.s.opts.Instance, SignIn: "?token=" + x.s.opts.Token})
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
