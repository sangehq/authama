package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/testcred"
	"github.com/sangehq/authama/src/internal/web"
)

func healthServer(t *testing.T, body string) (host string, port int) {
	t.Helper()
	// Some sandboxes cannot bind a socket; the probe is then not testable here.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind a port here:", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintln(w, body)
	}))
	_ = srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	h, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return h, n
}

// Every authama answering is found, with its pid when it says it; anything
// else on a port is not authama, and a closed port is skipped.
func TestStopAllFindsEveryAuthamaWebUI(t *testing.T) {
	host, current := healthServer(t, `{"app":"authama","instance":"aaaa","pid":4242,"version":"v0.1.0"}`)
	_, older := healthServer(t, `{"app":"authama","instance":"bbbb"}`)
	_, other := healthServer(t, `{"app":"something-else","instance":"cccc"}`)
	ln, err := net.Listen("tcp", host+":0")
	if err != nil {
		t.Skip("cannot bind a port here:", err)
	}
	closed := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	found := findServers(host, []int{current, older, other, closed}, "")
	if len(found) != 2 || found[0].PID != 4242 || found[0].Version != "v0.1.0" || found[1].PID != 0 {
		t.Fatalf("found = %+v", found)
	}
	// An older build that does not say its pid is reported with the command
	// that stops it, never guessed at.
	err = stopFound(found[1], "")
	if err == nil || !strings.Contains(err.Error(), "lsof -ti tcp:"+strconv.Itoa(older)) {
		t.Errorf("stopping an older build: %v", err)
	}
	if err := stopFound(foundServer{Addr: found[0].Addr, PID: os.Getpid()}, ""); err == nil {
		t.Error("stop-all would stop the process running it")
	}
	var out bytes.Buffer
	if failed := stopAll(&out, host, []int{older}, "", nil); failed != 1 || !strings.Contains(out.String(), "could not stop the web UI at") {
		t.Errorf("stopAll = %d, %q", failed, out.String())
	}
	// The home's record supplies the pid an older build does not say: here
	// this test's own pid, which stopFound refuses, proving the record's pid
	// was the one used.
	out.Reset()
	rec := &serveRecord{PID: os.Getpid(), Instance: "bbbb", Addr: found[1].Addr}
	if failed := stopAll(&out, host, []int{older}, "", rec); failed != 1 || !strings.Contains(out.String(), "it is this process") {
		t.Errorf("with the record: %q", out.String())
	}
}

// The handoff is read once and cleared, so no child of the new server
// inherits the token.
func TestTheHandoffIsReadOnceAndCleared(t *testing.T) {
	t.Setenv(envnames.ServeHandoff, `{"token":"tok","port":7822,"record":true,"addr":"127.0.0.1:7822","instance":"aaaa"}`)
	h := takeHandoff()
	if h == nil || h.Token != "tok" || h.Port != 7822 || !h.Record {
		t.Fatalf("handoff = %+v", h)
	}
	if _, ok := os.LookupEnv(envnames.ServeHandoff); ok {
		t.Error("the handoff was left in the environment")
	}
	t.Setenv(envnames.ServeHandoff, `{"token":"","port":0}`)
	if takeHandoff() != nil {
		t.Error("an empty handoff was taken")
	}
}

func TestStopAllProbesTheDefaultRangeAndTheConfiguredPort(t *testing.T) {
	ports := serverPorts(9000, &serveRecord{Addr: "127.0.0.1:7911"})
	if ports[0] != web.DefaultPort || len(ports) != web.DefaultPortAttempts+2 ||
		!slices.Contains(ports, 9000) || !slices.Contains(ports, 7911) {
		t.Errorf("ports = %v", ports)
	}
	if got := serverPorts(web.DefaultPort, nil); len(got) != web.DefaultPortAttempts {
		t.Errorf("the default port was probed twice: %v", got)
	}
}

// A restarted server never re-arms the sign-in token the old one printed:
// a used link stayed used only until the next rebuild (review R-SEC-05). It
// mints a fresh token, keeps the control key so the record stays valid, and
// hands the old token on only through Links, which hold unused ones.
func TestARestartMintsAFreshSignInToken(t *testing.T) {
	h := &serveHandoff{Token: testcred.Secret("printed"), Control: testcred.Secret("control")}
	token, control := serveTokens(h)
	if token == "" || token == h.Token {
		t.Errorf("restart sign-in token = %q, want a fresh one", token)
	}
	if control != h.Control {
		t.Errorf("control key = %q, want the old one kept", control)
	}
	token, control = serveTokens(nil)
	if token == "" || control == "" || token == control {
		t.Errorf("a first launch: token %q control %q", token, control)
	}
}

// The handoff this build writes, with no token since R-SEC-05, is taken by
// the next server; without this a restart quietly started from scratch.
func TestTheHandoffThisBuildWritesIsTaken(t *testing.T) {
	b, err := json.Marshal(serveHandoff{Control: "control-key", Port: 7822, Record: true, Addr: "127.0.0.1:7822", Instance: "aaaa"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envnames.ServeHandoff, string(b))
	h := takeHandoff()
	if h == nil || h.Control != "control-key" || h.Port != 7822 {
		t.Fatalf("handoff written without a token was not taken: %+v", h)
	}
}

// A restart runs the binary at this executable's path while it is there,
// else the authama PATH resolves, else it says why (found 2026-11-13: a
// server started from a checkout that then moved execed a missing file).
func TestARestartRunsTheBinaryOnPATHWhenItsOwnIsGone(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "authama")
	onPath := filepath.Join(dir, "elsewhere", "authama")
	for _, f := range []string{own, onPath} {
		if err := os.MkdirAll(filepath.Dir(f), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("bin"), 0o755); err != nil { //nolint:gosec // a test executable
			t.Fatal(err)
		}
	}
	look := func(string) (string, error) { return onPath, nil }
	if got, err := restartExecutable(own, look); err != nil || got != own {
		t.Errorf("own binary present: %q, %v", got, err)
	}
	gone := filepath.Join(dir, "moved-away", "authama")
	want, _ := filepath.EvalSymlinks(onPath)
	if got, err := restartExecutable(gone, look); err != nil || got != want {
		t.Errorf("own binary gone: %q, %v; want the one on PATH", got, err)
	}
	none := func(string) (string, error) { return "", exec.ErrNotFound }
	if _, err := restartExecutable(gone, none); err == nil || !strings.Contains(err.Error(), gone) || !strings.Contains(err.Error(), "PATH") {
		t.Errorf("nothing to run: %v; want the reason naming the old path and PATH", err)
	}
}

// The handoff's secrets (control key, sessions, form tokens) no longer ride
// in the environment, where /proc/<pid>/environ keeps them for the life of
// the process (R-SEC-21): the variable names an owner-only file, which the
// new server reads once and removes.
func TestTheHandoffTravelsInAnOwnerOnlyFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	control := testcred.Secret("control")
	entry, err := handoffEnv(serveHandoff{Control: control, Port: 7822, Addr: "127.0.0.1:7822", Instance: "aaaa"})
	if err != nil {
		t.Fatal(err)
	}
	name, value, _ := strings.Cut(entry, "=")
	if name != envnames.ServeHandoff {
		t.Fatalf("entry %q sets the wrong variable", name)
	}
	if strings.Contains(value, control) {
		t.Fatal("the control key is in the environment")
	}
	path, ok := strings.CutPrefix(value, handoffFilePrefix)
	if !ok {
		t.Fatalf("value %q names no file", value)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("handoff file mode = %v, want 0600", info.Mode().Perm())
	}
	t.Setenv(envnames.ServeHandoff, value)
	h := takeHandoff()
	if h == nil || h.Control != control || h.Port != 7822 {
		t.Fatalf("handoff from the file = %+v", h)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("the handoff file was left behind")
	}
}
