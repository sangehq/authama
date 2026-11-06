package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/services/mutation"
)

// A real ed25519 public key, so the fingerprint is computed from real bytes.
const fakeHostKey = "github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"

// fakeKeyscanOnPath puts an ssh-keyscan on PATH that prints out.
func fakeKeyscanOnPath(t *testing.T, out string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	dir := t.TempDir()
	body := strings.ReplaceAll(out, "'", `'\''`)
	script := "#!/bin/sh\nprintf '%s\\n' '" + body + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh-keyscan"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// waitFor waits until the job reaches one of the states.
func waitFor(t *testing.T, s *Server, id string, states ...string) jobs.View {
	t.Helper()
	j := s.jobs.Get(id)
	if j == nil {
		t.Fatalf("no job %s", id)
	}
	deadline := time.After(15 * time.Second)
	for {
		v, changed := j.View()
		for _, st := range states {
			if v.State == st {
				return v
			}
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("job still %s, want one of %v", v.State, states)
		}
	}
}

func knownHostsFile(t *testing.T, s *Server) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(s.opts.Engine.Home().SSHDir, "known_hosts"))
	return string(b)
}

// Pinning a host from its page is `knownhosts pin`: it scans the host, shows
// each key's fingerprint, and writes nothing until the person says yes.
func TestPinningAHostShowsItsKeysAndAsksFirst(t *testing.T) {
	fakeKeyscanOnPath(t, fakeHostKey)
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	if body := c.get("/hosts/gh").Body.String(); !strings.Contains(body, `value="knownhosts-pin"`) {
		t.Fatalf("the host page offers no pin:\n%s", body)
	}

	f := url.Values{"action": {"knownhosts-pin"}, "host": {"gh"}}
	rev := c.change("/jobs/review", f)
	if rev.Code != http.StatusOK || !strings.Contains(rev.Body.String(), "authama ssh knownhosts pin gh --yes") {
		t.Fatalf("review: %d\n%s", rev.Code, rev.Body.String())
	}
	start := c.change("/jobs", f)
	id := strings.TrimPrefix(start.Header().Get("Location"), "/jobs/")
	v := waitFor(t, s, id, jobs.Waiting, jobs.Done, jobs.Failed)
	if v.State != jobs.Waiting || v.Question == nil || !strings.Contains(v.Question.Text, "SHA256:") {
		t.Fatalf("the job should stop on the fingerprints, got %s %+v %q", v.State, v.Question, v.Err)
	}
	if strings.Contains(knownHostsFile(t, s), "AAAAC3") {
		t.Fatal("known_hosts was written before the person answered")
	}
	if page := c.get("/jobs/" + id).Body.String(); !strings.Contains(page, "Trust these keys") || !strings.Contains(page, "Don&#39;t trust them") {
		t.Errorf("the question should be answered in its own words:\n%s", page)
	}
	if !s.jobs.Answer(id, true) {
		t.Fatal("the job did not take the answer")
	}
	v = waitFor(t, s, id, jobs.Done, jobs.Failed)
	if v.State != jobs.Done {
		t.Fatalf("job = %s %q", v.State, v.Err)
	}
	if !strings.Contains(knownHostsFile(t, s), "AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl") {
		t.Errorf("the trusted key is not in known_hosts:\n%s", knownHostsFile(t, s))
	}
	if len(liveOps(t, s)) == 0 {
		t.Error("the pin was not recorded in the history")
	}
}

// Saying no is the CLI's "n": nothing is written, and the job says so.
func TestDecliningAPinWritesNothing(t *testing.T) {
	fakeKeyscanOnPath(t, fakeHostKey)
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	start := c.change("/jobs", url.Values{"action": {"knownhosts-pin"}, "host": {"gh"}})
	id := strings.TrimPrefix(start.Header().Get("Location"), "/jobs/")
	waitFor(t, s, id, jobs.Waiting)
	s.jobs.Answer(id, false)
	v := waitFor(t, s, id, jobs.Done, jobs.Failed)
	if v.State != jobs.Done || !strings.Contains(strings.Join(v.Lines, "\n"), "pinned 0 host key(s)") {
		t.Errorf("job = %s %q\n%s", v.State, v.Err, strings.Join(v.Lines, "\n"))
	}
	if strings.Contains(knownHostsFile(t, s), "AAAAC3") {
		t.Error("a declined key was written")
	}
}

// The web pins manifest hosts only: a host it does not know is refused at
// review, so no free hostname or port reaches ssh-keyscan from a form.
func TestPinTakesOnlyAManifestHost(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, host := range []string{"", "nope", "-oProxyCommand=x", "../gh"} {
		rec := c.change("/jobs/review", url.Values{"action": {"knownhosts-pin"}, "host": {host}})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("host %q: %d", host, rec.Code)
		}
	}
}

// Init from the web is `knownhosts init`, with the trust-on-first-use notice
// the CLI confirms with, and runs the same service: its report is the CLI's.
func TestKnownHostsInitFromTheWebIsTheCommand(t *testing.T) {
	fakeKeyscanOnPath(t, fakeHostKey)
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	if body := c.get("/hosts").Body.String(); !strings.Contains(body, `value="knownhosts-init"`) {
		t.Fatalf("the hosts page offers no init:\n%s", body)
	}
	f := url.Values{"action": {"knownhosts-init"}, "all": {"1"}}
	rev := c.change("/jobs/review", f)
	body := rev.Body.String()
	if rev.Code != http.StatusOK || !strings.Contains(body, "authama ssh knownhosts init --all --yes") ||
		!strings.Contains(body, "trust on first use") {
		t.Fatalf("review: %d\n%s", rev.Code, body)
	}
	if rec := c.change("/jobs/review", url.Values{"action": {"knownhosts-init"}, "profile": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown profile was accepted: %d", rec.Code)
	}
	start := c.change("/jobs", f)
	v := waitFor(t, s, strings.TrimPrefix(start.Header().Get("Location"), "/jobs/"), jobs.Done, jobs.Failed)
	if v.State != jobs.Done || !strings.Contains(strings.Join(v.Lines, "\n"), "knownhosts init: 1 profile(s)") {
		t.Errorf("job = %s %q\n%s", v.State, v.Err, strings.Join(v.Lines, "\n"))
	}
}

// Scanning a host and writing the sandbox's own known_hosts reaches no
// server's state, so dev mode offers both, as the CLI runs both there.
func TestDevModeStillPinsHostKeys(t *testing.T) {
	p, _ := fixtureHome(t)
	p.DevRoot = filepath.Dir(p.SSHDir)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil)})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	for _, f := range []url.Values{
		{"action": {"knownhosts-pin"}, "host": {"gh"}},
		{"action": {"knownhosts-init"}, "all": {"1"}},
	} {
		if rec := c.change("/jobs/review", f); rec.Code != http.StatusOK {
			t.Errorf("%s in dev mode: %d\n%s", f.Get("action"), rec.Code, rec.Body.String())
		}
	}
}
