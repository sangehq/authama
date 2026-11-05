package cli

import (
	"os"
	"strings"
	"testing"
	"time"
)

// A server already running for the home is opened again, not doubled: the
// second launch binds nothing. --new starts another anyway, a record whose
// server no longer answers is ignored, and the running server's record is
// removed when it stops.
func TestServeReusesTheRunningServerForTheHome(t *testing.T) {
	p := editHome(t)
	rec := serveRecord{PID: 4242, Addr: "127.0.0.1:7822", URL: "http://127.0.0.1:7822/?token=x", Instance: "abc", Started: time.Now()}
	if err := writeServeRecord(p, rec); err != nil {
		t.Fatal(err)
	}
	prev := serveAlive
	t.Cleanup(func() { serveAlive = prev })
	serveAlive = func(r *serveRecord) bool { return r.Instance == "abc" }

	tried, stop := fakeServe(t)
	out, err := runServe(t, stop)
	if err != nil || !strings.Contains(out, "already running for this home (pid 4242)") || len(*tried) != 0 {
		t.Fatalf("second launch: %v, bound %v\n%s", err, *tried, out)
	}
	if out, _ := runServe(t, stop, "--status"); !strings.Contains(out, "pid 4242") {
		t.Fatalf("--status:\n%s", out)
	}

	// --new starts one beside it; it records itself, and removes only its own
	// record when it stops.
	tried, stop = fakeServe(t)
	out, err = runServe(t, stop, "--new")
	if err != nil || len(*tried) == 0 || !strings.Contains(out, "press Ctrl-C to stop") {
		t.Fatalf("--new: %v, bound %v\n%s", err, *tried, out)
	}
	// --new recorded itself over the old record, and removed it on stop.
	if r := readServeRecord(p); r != nil {
		t.Fatalf("a record is left after --new stopped: %+v", r)
	}

	// A record whose server is gone is stale: the next launch starts a server.
	if err := writeServeRecord(p, rec); err != nil {
		t.Fatal(err)
	}
	serveAlive = func(*serveRecord) bool { return false }
	tried, stop = fakeServe(t)
	out, err = runServe(t, stop)
	if err != nil || len(*tried) == 0 {
		t.Fatalf("stale record: %v, bound %v\n%s", err, *tried, out)
	}
	if _, err := os.Stat(serveRecordPath(p)); err == nil {
		t.Fatal("the server left its record behind when it stopped")
	}
	if out, _ := runServe(t, stop, "--status"); !strings.Contains(out, "no authama web UI is running") {
		t.Fatalf("--status with nothing running:\n%s", out)
	}
}
