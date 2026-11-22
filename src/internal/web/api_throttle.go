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
	// apiFailAll is a ceiling on bad tokens from every address together in
	// a window: an attacker rotating addresses (cheap on IPv6) evicts
	// per-address windows, but not this one (audit A-SEC-10).
	apiFailAll = 300
)

type failWindow struct {
	start time.Time
	n     int
}

// failLimiter counts failed API sign-ins per remote host.
type failLimiter struct {
	mu     sync.Mutex
	byHost map[string]*failWindow
	all    failWindow // every address together
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
	if now.Sub(l.all.start) >= apiFailWindow {
		l.all = failWindow{start: now}
	}
	l.all.n++
}

// blocked returns how long host must wait, or 0 when it may try.
func (l *failLimiter) blocked(host string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.all.n >= apiFailAll {
		if left := l.all.start.Add(apiFailWindow).Sub(now); left > 0 {
			return left
		}
	}
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

// remoteHost is the address a request came from, without its port. An
// IPv6 address counts as its /64, the block one host is usually given, so
// rotating addresses within it does not start a fresh window each time
// (audit A-SEC-10).
func remoteHost(r *http.Request) string {
	h := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		h = host
	}
	if ip := net.ParseIP(h); ip != nil && ip.To4() == nil && !ip.IsLoopback() {
		return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
	}
	return h
}

// retryAfter writes a 429 naming when to try again.
func retryAfter(w http.ResponseWriter, wait time.Duration) {
	secs := int((wait + time.Second - 1) / time.Second)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	apiError(w, http.StatusTooManyRequests, "too_many_failures",
		"Too many requests with a bad API token from this address. Wait "+strconv.Itoa(secs)+"s and send a valid one.")
}
