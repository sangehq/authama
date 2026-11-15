package cli

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/web"
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
	prev, prevLink := serveAlive, serveNewLink
	t.Cleanup(func() { serveAlive, serveNewLink = prev, prevLink })
	serveAlive = func(r *serveRecord) bool { return r.Instance == "abc" }
	// The running server is asked for a new link: the one it printed works once.
	fresh := "http://127.0.0.1:7822/?token=" + web.NewToken()
	serveNewLink = func(r *serveRecord) (string, error) { return fresh, nil }

	tried, stop := fakeServe(t)
	out, err := runServe(t, stop)
	if err != nil || !strings.Contains(out, "already running for this home (pid 4242): "+fresh) || len(*tried) != 0 {
		t.Fatalf("second launch: %v, bound %v\n%s", err, *tried, out)
	}
	if out, _ := runServe(t, stop, "--status"); !strings.Contains(out, "pid 4242: http://127.0.0.1:7822/\n") || strings.Contains(out, "token=") {
		t.Fatalf("--status shows the address without a token:\n%s", out)
	}
	// A server that gives no link (one from an older build) is named, with the
	// way out, and nothing is started beside it.
	serveNewLink = func(r *serveRecord) (string, error) { return "", errors.New("its record has no control key") }
	tried, stop = fakeServe(t)
	if _, err := runServe(t, stop); err == nil || !strings.Contains(err.Error(), "serve --restart") || len(*tried) != 0 {
		t.Fatalf("a server that gives no link: %v, bound %v", err, *tried)
	}
	serveNewLink = func(r *serveRecord) (string, error) { return fresh, nil }

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
