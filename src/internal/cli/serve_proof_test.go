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
			return jsonReply(http.StatusOK, map[string]string{"sign_in": "?token=fresh"})
		}
		return jsonReply(http.StatusNotFound, nil)
	})
	t.Cleanup(func() { healthClient.Transport = old })
	return got
}

// The control key goes only to a server that proves it holds the same key:
// something else that took a dead server's port, answering with the public
// instance id, used to receive it (review R-SEC-08).
func TestTheControlKeyGoesOnlyToAServerThatProvesIt(t *testing.T) {
	rec := &serveRecord{Addr: "127.0.0.1:7822", Instance: "inst", Control: "control-key"}

	sent := fakeServer(t, "") // an impostor: right instance, no proof
	if rec.checkAlive() {
		t.Error("an impostor with the instance id passed as the recorded server")
	}
	if _, err := rec.newLink(); err == nil || *sent {
		t.Errorf("newLink to an impostor: err=%v, control key sent=%v", err, *sent)
	}

	sent = fakeServer(t, "another-key") // a server with a different key
	if _, err := rec.newLink(); err == nil || *sent {
		t.Errorf("newLink to a server with another key: err=%v, sent=%v", err, *sent)
	}

	sent = fakeServer(t, "control-key") // the real one
	if !rec.checkAlive() {
		t.Fatal("the recorded server was not recognised")
	}
	if link, err := rec.newLink(); err != nil || !*sent || !strings.HasSuffix(link, "?token=fresh") {
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

	_ = fakeServer(t, "") // the impostor again
	rec := &serveRecord{PID: child.Process.Pid, Addr: "127.0.0.1:7822", Instance: "inst", Control: "control-key"}
	if err := rec.stop(); err == nil {
		t.Error("stop went ahead against an unproven server")
	}
	select {
	case <-done:
		t.Fatal("stop signalled a process an impostor pointed at")
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
	case <-done:
		t.Fatal("stop-all signalled a decoy process")
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
