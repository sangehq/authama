package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPortHolder: a taken port is named by asking its /healthz. The
// sange-family tools answer it: imprimatur and authama with {"app": ...},
// sange with plain "ok".
func TestPortHolder(t *testing.T) {
	serve := func(body string) string {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(s.Close)
		return strings.TrimPrefix(s.URL, "http://")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := ln.Addr().String()
	_ = ln.Close()
	cases := []struct{ name, addr, want string }{
		{"imprimatur", serve(`{"app":"imprimatur","instance":"x"}`), "imprimatur"},
		{"sange", serve("ok\n"), "sange"},
		{"authama", serve(`{"app":"authama","instance":"x"}`), "another authama web UI"},
		{"unknown", serve("<html>hi</html>"), "another program"},
		{"nothing", free, "another program"},
	}
	for _, c := range cases {
		if got := PortHolder(c.addr); got != c.want {
			t.Errorf("%s: PortHolder = %q, want %q", c.name, got, c.want)
		}
	}
	if h := PortTakenHint("127.0.0.1:7822"); !strings.Contains(h, "lsof -nP -iTCP:7822 -sTCP:LISTEN") {
		t.Errorf("hint has no lsof command: %q", h)
	}
}
