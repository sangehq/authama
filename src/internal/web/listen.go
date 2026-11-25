package web

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
)

// DefaultPort is where serve listens unless told otherwise.
const DefaultPort = 7822

// DefaultPortAttempts is how many ports from DefaultPort up `serve --stop-all`
// probes for a running web UI. Builds before the OS-picked fallback took the
// next free port in that range, so their servers can still be there. Listen
// no longer scans upward; this is the probe's width only.
const DefaultPortAttempts = 20

// ListenFunc is net.Listen's signature, injectable so the port fallback can be
// tested without binding a socket.
type ListenFunc func(network, address string) (net.Listener, error)

// Listen binds host:port.
//
// When pinned is false and the port is in use, it asks the OS for any free
// port and returns that: the default port being taken by another process is
// ordinary, and the caller prints the URL actually bound. It does not try the
// ports above: the sange-family tools have fixed ports side by side (sange
// 7821, authama 7822, imprimatur 7823), and a port the OS picks comes from its
// ephemeral range (49152 and up on macOS and Windows, 32768 and up on Linux),
// which holds none of them. When pinned is true -- the user asked for this
// port -- a taken port is an error, because silently serving elsewhere would
// leave whatever they pointed at this port talking to something else. Any
// error other than "address in use" is returned at once.
//
// attempts no longer counts ports: attempts < 1 means one try with no
// fallback, and any other value takes the OS-picked fallback. The parameter
// stays so callers compile.
func Listen(listen ListenFunc, host string, port int, pinned bool, attempts int) (net.Listener, error) {
	if listen == nil {
		listen = net.Listen
	}
	ln, err := listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		return ln, nil
	}
	if !IsAddrInUse(err) {
		return nil, err
	}
	if pinned {
		return nil, fmt.Errorf("port %d is already in use; pick another with --port, or omit --port to take a free one: %w", port, err)
	}
	if port == 0 || attempts < 1 {
		return nil, fmt.Errorf("port %d is in use: %w", port, err)
	}
	// Ask the OS for any free port: the caller prints the address actually
	// bound, so nothing is lost but a memorable number.
	if ln, anyErr := listen("tcp", net.JoinHostPort(host, "0")); anyErr == nil {
		return ln, nil
	}
	return nil, fmt.Errorf("port %d is in use and no free port was given: %w", port, err)
}

// IsAddrInUse reports whether a listen error means the port is taken.
func IsAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	// Windows reports WSAEADDRINUSE, which does not unwrap to EADDRINUSE.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}

// IsLoopback reports whether host only accepts connections from this machine:
// localhost, or an address in 127.0.0.0/8 or ::1.
func IsLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// AllowedHosts is the set of Host header values the server answers to.
//
// For a loopback bind that is the three spellings of this machine at the bound
// port. Anything else is refused, which is the DNS-rebinding defence: a page
// on evil.example that rebinds its name to 127.0.0.1 still sends
// Host: evil.example. With a remote bind the bound host and any explicitly
// allowed names are added.
func AllowedHosts(bindHost string, port int, extra []string) []string {
	p := strconv.Itoa(port)
	out := []string{
		net.JoinHostPort("127.0.0.1", p),
		net.JoinHostPort("localhost", p),
		net.JoinHostPort("::1", p),
	}
	if !IsLoopback(bindHost) && bindHost != "" && bindHost != "0.0.0.0" && bindHost != "::" {
		out = append(out, net.JoinHostPort(bindHost, p))
	}
	for _, h := range extra {
		if _, _, err := net.SplitHostPort(h); err != nil {
			h = net.JoinHostPort(h, p)
		}
		out = append(out, h)
	}
	return out
}
