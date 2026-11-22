package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/testcred"
	"github.com/sangehq/authama/src/internal/web"
)

// roundTrip answers requests in-process, so these tests need no port.
type roundTrip func(*http.Request) *http.Response

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r), nil }

func jsonReply(code int, v any) *http.Response {
	b, _ := json.Marshal(v)
	rec := httptest.NewRecorder()
	rec.WriteHeader(code)
	_, _ = rec.Write(b)
	return rec.Result()
}

// fakeServer answers /healthz as instance "inst" (with a proof under key when
// key is set) and records whether the control key reached LinkPath.
func fakeServer(t *testing.T, key string) *bool {
	t.Helper()
	got := new(bool)
	old := healthClient.Transport
	healthClient.Transport = roundTrip(func(r *http.Request) *http.Response {
		switch r.URL.Path {
		case "/healthz":
			h := map[string]string{"app": "authama", "instance": "inst"}
			if c := r.URL.Query().Get("challenge"); c != "" && key != "" {
				h["proof"] = web.HealthProof(key, c)
			}
			return jsonReply(http.StatusOK, h)
		case web.LinkPath:
			if r.Header.Get(web.ControlHeader) != "" {
				*got = true
			}
			reply := map[string]string{"sign_in": "?token=fresh"}
			if c := r.Header.Get(web.LinkChallengeHeader); c != "" && key != "" {
				if r.Header.Get(web.LinkProofHeader) != web.LinkCallerProof(key, "inst", c) {
					return jsonReply(http.StatusForbidden, nil)
				}
				reply["proof"] = web.LinkProof(key, c, reply["sign_in"])
			}
			return jsonReply(http.StatusOK, reply)
		}
		return jsonReply(http.StatusNotFound, nil)
	})
	t.Cleanup(func() { healthClient.Transport = old })
	return got
}

// The control key never goes to the server at all: a new link is asked for
// in one request that proves the key, and taken only with the server's proof
// under it. Something else that took a dead server's port, answering with
// the public instance id, used to receive the key (review R-SEC-08), and
// still could between the /healthz proof and the link request (audit P-02).
func TestTheControlKeyGoesOnlyToAServerThatProvesIt(t *testing.T) {
	key, other := testcred.Secret("control"), testcred.Secret("other")
	rec := &serveRecord{Addr: "127.0.0.1:7822", Instance: "inst", Control: key}

	sent := fakeServer(t, "") // an impostor: right instance, no proof
	if rec.checkAlive() {
		t.Error("an impostor with the instance id passed as the recorded server")
	}
	if _, err := rec.newLink(); err == nil || *sent {
		t.Errorf("newLink to an impostor: err=%v, control key sent=%v", err, *sent)
	}

	sent = fakeServer(t, other) // a server with a different key
	if _, err := rec.newLink(); err == nil || *sent {
		t.Errorf("newLink to a server with another key: err=%v, sent=%v", err, *sent)
	}

	sent = fakeServer(t, key) // the real one
	if !rec.checkAlive() {
		t.Fatal("the recorded server was not recognised")
	}
	if link, err := rec.newLink(); err != nil || *sent || !strings.HasSuffix(link, "?token=fresh") {
		t.Errorf("newLink to the recorded server: %q %v sent=%v", link, err, *sent)
	}

	// A record from an older build has no key; the instance id is all it has.
	_ = fakeServer(t, "")
	if !(&serveRecord{Addr: rec.Addr, Instance: "inst"}).checkAlive() {
		t.Error("an older record without a control key is no longer recognised")
	}
}

// stop never signals the recorded pid unless the server at the address
// proves it is that server: the pid in the record may by now be anything.
func TestStopSignalsOnlyAProvenServer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX child process")
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Skip("no sleep here:", err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	t.Cleanup(func() { _ = child.Process.Kill() })

	key := testcred.Secret("control")
	_ = fakeServer(t, "") // the impostor again
	rec := &serveRecord{PID: child.Process.Pid, Addr: "127.0.0.1:7822", Instance: "inst", Control: key}
	if err := rec.stop(); err == nil {
		t.Error("stop went ahead against an unproven server")
	}
	select {
	case err := <-done:
		t.Fatalf("stop signalled a process an impostor pointed at (%v)", err)
	case <-time.After(300 * time.Millisecond):
	}
}

// --stop-all signals a pid another server reports only when that pid is an
// authama process: a listener could name any process of yours (R-SEC-07).
func TestStopAllNeverSignalsAProcessThatIsNotAuthama(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX child process")
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Skip("no sleep here:", err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	t.Cleanup(func() { _ = child.Process.Kill() })

	_ = fakeServer(t, "")
	err := stopFound(foundServer{Addr: "127.0.0.1:7822", Instance: "inst", PID: child.Process.Pid}, "")
	if err == nil || !strings.Contains(err.Error(), "not an authama process") {
		t.Errorf("stopFound on a decoy pid: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("stop-all signalled a decoy process (%v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	// The lookup itself, where ps may run (a sandbox can refuse it; the
	// refusal above then held because an unknown process is never signalled).
	name, err := processName(child.Process.Pid)
	if err != nil && strings.Contains(err.Error(), "operation not permitted") {
		t.Skip("ps cannot run here:", err)
	}
	if err != nil || !strings.Contains(name, "sleep") {
		t.Errorf("processName(sleep) = %q, %v", name, err)
	}
	if ok, _ := isAuthamaProcess(child.Process.Pid); ok {
		t.Error("sleep was taken for authama")
	}
}

// A listener on a probed port names the pid --stop-all signals. Being named
// authama is not enough: a running `authama ssh rotate`, interrupted
// half-way, is an authama process too. Only one serving the web UI (`serve`,
// or bare authama) is stopped (audit A-SEC-02, 2026-11-16).
func TestStopAllSignalsOnlyAProcessServingTheWebUI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX child process")
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Skip("no sleep here:", err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	t.Cleanup(func() { _ = child.Process.Kill() })

	oldName, oldArgs := processName, processArgs
	t.Cleanup(func() { processName, processArgs = oldName, oldArgs })
	processName = func(int) (string, error) { return "/usr/local/bin/authama", nil }
	processArgs = func(int) ([]string, error) {
		return []string{"/usr/local/bin/authama", "ssh", "rotate", "--all", "-y"}, nil
	}
	_ = fakeServer(t, "")
	err := stopFound(foundServer{Addr: "127.0.0.1:7822", Instance: "inst", PID: child.Process.Pid}, "")
	if err == nil || !strings.Contains(err.Error(), "not serving") {
		t.Errorf("stopFound on an authama rotation: %v", err)
	}
	select {
	case err := <-done:
		t.Fatalf("stop-all interrupted an authama process that was not a web UI (%v)", err)
	case <-time.After(300 * time.Millisecond):
	}

	for _, tc := range []struct {
		args  []string
		serve bool
	}{
		{[]string{"/usr/local/bin/authama", "serve", "--no-open"}, true},
		{[]string{"authama", "--read-only", "serve"}, true},
		{[]string{"/opt/authama"}, true},
		{[]string{"authama", "--no-open"}, true},
		{[]string{"authama", "ssh", "rotate"}, false},
		{[]string{"authama", "mcp"}, false},
		{[]string{}, false},
	} {
		if got := servesWebUI(tc.args); got != tc.serve {
			t.Errorf("servesWebUI(%q) = %v, want %v", tc.args, got, tc.serve)
		}
	}
}

// keyedServers answers /healthz on probeClient as the servers named, each
// instance with its own control key, giving its pid only to a caller that
// proves that key, as a real server does since audit A-SEC-09. One with no
// key is a decoy (or an older build): it offers a pid to anyone.
func keyedServers(t *testing.T, servers map[string]struct {
	instance, key string
	pid           int
}) {
	t.Helper()
	old := probeClient.Transport
	probeClient.Transport = roundTrip(func(r *http.Request) *http.Response {
		srv, ok := servers[r.URL.Host]
		if !ok || r.URL.Path != "/healthz" {
			return jsonReply(http.StatusNotFound, nil)
		}
		h := map[string]any{"app": "authama", "instance": srv.instance}
		if srv.key == "" {
			h["pid"] = srv.pid
		}
		if c := r.URL.Query().Get("challenge"); c != "" && srv.key != "" {
			h["proof"] = web.HealthProof(srv.key, c)
			if r.URL.Query().Get("caller") == web.HealthCallerProof(srv.key, srv.instance, c) {
				h["pid"], h["version"] = srv.pid, "v9.9.9"
			}
		}
		return jsonReply(http.StatusOK, h)
	})
	t.Cleanup(func() { probeClient.Transport = old })
}

// --stop-all learns a server's pid only by proving a control key it holds:
// this home's server is found with its pid; another home's is found without
// one and is reported with the commands that stop it, never signalled (audit
// A-SEC-09).
func TestStopAllLearnsThePidOnlyOfAServerItHoldsTheKeyFor(t *testing.T) {
	mine, theirs := testcred.Secret("mine"), testcred.Secret("theirs")
	keyedServers(t, map[string]struct {
		instance, key string
		pid           int
	}{
		"127.0.0.1:7822": {"inst-mine", mine, 4242},
		"127.0.0.1:7823": {"inst-theirs", theirs, 4343},
		"127.0.0.1:7824": {"inst-decoy", "", 4444},
	})
	found := findServers("127.0.0.1", []int{7822, 7823, 7824}, "", []string{mine})
	if len(found) != 3 {
		t.Fatalf("found = %+v", found)
	}
	if found[0].PID != 4242 || found[0].Key != mine {
		t.Errorf("this home's server: %+v, want its pid and key", found[0])
	}
	if found[1].PID != 0 || found[1].Key != "" || found[1].Version != "" {
		t.Errorf("another home's server gave away %+v", found[1])
	}
	if found[2].PID != 0 {
		t.Errorf("a pid offered without a proof was taken: %+v", found[2])
	}
	err := stopFound(found[1], "")
	if err == nil || !strings.Contains(err.Error(), "serve --stop") || !strings.Contains(err.Error(), "lsof -ti tcp:7823") {
		t.Errorf("stopping another home's server: %v", err)
	}
	if none := findServers("127.0.0.1", []int{7822}, "", nil); len(none) != 1 || none[0].PID != 0 {
		t.Errorf("with no key held: %+v", none)
	}
}
