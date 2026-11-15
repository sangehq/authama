package web

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// A caller that sends apiFailLimit bad API tokens within apiFailWindow is
// answered 429 until the window that began with its first failure passes
// (R-SEC-19). Tokens are 256-bit, so this is hardening, not the defence: it
// keeps a misbehaving script from hammering the token store.
const (
	apiFailLimit  = 10
	apiFailWindow = time.Minute
	// apiFailHosts bounds the table, so a flood from many addresses cannot
	// grow it without limit; the oldest window is dropped first.
	apiFailHosts = 1024
)

type failWindow struct {
	start time.Time
	n     int
}

// failLimiter counts failed API sign-ins per remote host.
type failLimiter struct {
	mu     sync.Mutex
	byHost map[string]*failWindow
}

// fail records one failed sign-in from host at now.
func (l *failLimiter) fail(host string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.byHost == nil {
		l.byHost = map[string]*failWindow{}
	}
	w := l.byHost[host]
	if w == nil || now.Sub(w.start) >= apiFailWindow {
		if w == nil && len(l.byHost) >= apiFailHosts {
			l.dropOldest()
		}
		w = &failWindow{start: now}
		l.byHost[host] = w
	}
	w.n++
}

// blocked returns how long host must wait, or 0 when it may try.
func (l *failLimiter) blocked(host string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.byHost[host]
	if w == nil || w.n < apiFailLimit {
		return 0
	}
	if left := w.start.Add(apiFailWindow).Sub(now); left > 0 {
		return left
	}
	delete(l.byHost, host)
	return 0
}

func (l *failLimiter) dropOldest() {
	var oldest string
	for k, v := range l.byHost {
		if oldest == "" || v.start.Before(l.byHost[oldest].start) {
			oldest = k
		}
	}
	delete(l.byHost, oldest)
}

// remoteHost is the address a request came from, without its port.
func remoteHost(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

// retryAfter writes a 429 naming when to try again.
func retryAfter(w http.ResponseWriter, wait time.Duration) {
	secs := int((wait + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	apiError(w, http.StatusTooManyRequests, "too_many_failures",
		"Too many requests with a bad API token from this address. Wait "+strconv.Itoa(secs)+"s and send a valid one.")
}
