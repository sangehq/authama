package web

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

// fakeListener is a bound listener for a fake ListenFunc.
type fakeListener struct{ addr string }

func (f fakeListener) Accept() (net.Conn, error) { return nil, errors.New("fake") }
func (f fakeListener) Close() error              { return nil }
func (f fakeListener) Addr() net.Addr            { a, _ := net.ResolveTCPAddr("tcp", f.addr); return a }

// busyUntil binds only the addresses free says are free; everything else is
// in use. It records what was asked for.
func busyUntil(free map[string]bool, asked *[]string) ListenFunc {
	return func(_, addr string) (net.Listener, error) {
		*asked = append(*asked, addr)
		if free[addr] {
			return fakeListener{addr: strings.Replace(addr, ":0", ":54321", 1)}, nil
		}
		return nil, &net.OpError{Op: "listen", Err: syscall.EADDRINUSE}
	}
}

func TestListenTakesTheNextFreePort(t *testing.T) {
	var asked []string
	ln, err := Listen(busyUntil(map[string]bool{"127.0.0.1:7824": true}, &asked), "127.0.0.1", 7822, false, 20)
	if err != nil || ln.Addr().(*net.TCPAddr).Port != 7824 || len(asked) != 3 {
		t.Fatalf("got %v %v after %v", ln, err, asked)
	}
}

// Every preferred port taken - twenty servers left running, say - is not a
// reason to fail: the OS picks a free port, and the caller prints it.
func TestListenFallsBackToAnyFreePortWhenTheRangeIsFull(t *testing.T) {
	var asked []string
	ln, err := Listen(busyUntil(map[string]bool{"127.0.0.1:0": true}, &asked), "127.0.0.1", 7822, false, 20)
	if err != nil {
		t.Fatalf("a full range failed: %v (asked %v)", err, asked)
	}
	if got := ln.Addr().(*net.TCPAddr).Port; got != 54321 || asked[len(asked)-1] != "127.0.0.1:0" || len(asked) != 21 {
		t.Fatalf("bound %d after %v", got, asked)
	}
}

// A pinned port is the user's choice: taken means an error, never another port.
func TestAPinnedPortIsNeverReplaced(t *testing.T) {
	var asked []string
	_, err := Listen(busyUntil(map[string]bool{"127.0.0.1:0": true, "127.0.0.1:7823": true}, &asked), "127.0.0.1", 7822, true, 20)
	if err == nil || len(asked) != 1 {
		t.Fatalf("a pinned, taken port: %v after %v", err, asked)
	}
}
