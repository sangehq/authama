package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A server whose executable was rebuilt after it started says so on every
// page, and one whose executable is unchanged says nothing.
func TestAServerOlderThanItsBinarySaysSo(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	exe := filepath.Join(t.TempDir(), "authama")
	if err := os.WriteFile(exe, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	s.binary = binaryWatch{path: exe, started: time.Now().Add(time.Hour)}
	if body := c.get("/hosts").Body.String(); strings.Contains(body, "data-rebuilt") {
		t.Fatal("an unchanged binary showed the banner")
	}

	s.binary = binaryWatch{path: exe, started: time.Now().Add(-time.Hour)}
	body := c.get("/hosts").Body.String()
	if !strings.Contains(body, "data-rebuilt") || !strings.Contains(body, exe) || !strings.Contains(body, "authama serve") {
		t.Fatal("a rebuilt binary did not show the banner")
	}

	// A binary removed since the start is a change too.
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	s.binary = binaryWatch{path: exe, started: time.Now().Add(time.Hour)}
	if !strings.Contains(c.get("/hosts").Body.String(), "data-rebuilt") {
		t.Fatal("a removed binary did not show the banner")
	}
}
