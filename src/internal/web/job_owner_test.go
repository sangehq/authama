package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/jobs"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

// asking starts a job owned by c's session that waits on a question, as a
// rotation waits for a paste, and returns its id.
func asking(t *testing.T, c *client) string {
	t.Helper()
	sess := c.s.sessions.get(c.cookie.Value, c.s.now())
	j, err := c.s.jobs.StartFor(sess.owner, c.s.opts.Engine.Home(),
		mutation.Scope{Command: "rotate", Args: []string{"work/work_gh-ed25519", "--yes"}, Source: history.SourceWeb},
		func(j *jobs.Job) error {
			if !j.Ask("Pasted it?", "ssh-ed25519 AAAA test") {
				return jobs.ErrCancelled
			}
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	deadline := time.After(5 * time.Second)
	for {
		now, changed := j.View()
		if now.State == jobs.Waiting {
			return v.ID
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("the job never asked: %s", now.State)
		}
	}
}

// A job belongs to the session that started it (review R-GAP-06, owner:
// "jobs per session"). Another signed-in session sees its page read-only:
// no answer or cancel controls, and a 403 that says why if it posts anyway.
func TestOnlyTheSessionThatStartedAJobAnswersOrCancelsIt(t *testing.T) {
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	owner, other := signIn(t, s), signIn(t, s)
	id := asking(t, owner)

	page := other.get("/jobs/" + id).Body.String()
	for _, banned := range []string{"/jobs/" + id + "/answer", "/jobs/" + id + "/cancel"} {
		if strings.Contains(page, banned) {
			t.Errorf("another session's page offers %s", banned)
		}
	}
	if !strings.Contains(page, `id="job-not-yours"`) {
		t.Error("another session's page does not say the job is read-only to it")
	}
	for _, path := range []string{"/answer", "/cancel"} {
		rec := other.change("/jobs/"+id+path, url.Values{"answer": {"yes"}})
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "started from another sign-in") {
			t.Errorf("another session's %s: %d\n%.300s", path, rec.Code, rec.Body.String())
		}
	}
	if v, _ := s.jobs.Get(id).View(); v.State != jobs.Waiting {
		t.Fatalf("another session moved the job on: %s", v.State)
	}

	mine := owner.get("/jobs/" + id).Body.String()
	if !strings.Contains(mine, "/jobs/"+id+"/answer") || !strings.Contains(mine, "/jobs/"+id+"/cancel") {
		t.Error("the owner's page has no answer or cancel controls")
	}
	if rec := owner.change("/jobs/"+id+"/answer", url.Values{"answer": {"yes"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("the owner's answer: %d\n%.300s", rec.Code, rec.Body.String())
	}
	if v := waitJob(t, s, id); v.State != jobs.Done {
		t.Errorf("job = %s %q after the owner's yes", v.State, v.Err)
	}
}

// Signing out and in again is a new session, which is not the owner.
func TestSigningInAgainDoesNotOwnTheJob(t *testing.T) {
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	id := asking(t, c)
	sess := s.sessions.get(c.cookie.Value, s.now())
	if rec := c.post("/signout", csrfField+"="+sess.csrf); rec.Code >= 400 {
		t.Fatalf("sign out: %d", rec.Code)
	}
	again := signIn(t, s)
	if rec := again.change("/jobs/"+id+"/cancel", url.Values{}); rec.Code != http.StatusForbidden {
		t.Errorf("a new sign-in cancelled the job: %d", rec.Code)
	}
	if !strings.Contains(again.get("/jobs/"+id).Body.String(), "signed in again") {
		t.Error("the page does not say a new sign-in is not the owner")
	}
	s.jobs.Cancel(id)
	waitJob(t, s, id)
}

// A job a web form started records that session as its owner.
func TestAStartedJobIsOwnedByItsSession(t *testing.T) {
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c, other := signIn(t, s), signIn(t, s)
	key := fixtureKey(t, c)
	start := c.change("/jobs", url.Values{"action": {"rollback"}, "key": {key}})
	id := strings.TrimPrefix(start.Header().Get("Location"), "/jobs/")
	j := s.jobs.Get(id)
	if j == nil {
		t.Fatalf("no job: %d %s", start.Code, start.Header().Get("Location"))
	}
	waitJob(t, s, id)
	if !j.OwnedBy(s.sessions.get(c.cookie.Value, s.now()).owner) {
		t.Error("the job is not owned by the session that started it")
	}
	if j.OwnedBy(s.sessions.get(other.cookie.Value, s.now()).owner) {
		t.Error("the job is owned by another session")
	}
}

// Answering and cancelling are writes, so they need the PIN step-up every
// other write needs: within five minutes of the PIN they go through, after
// that the page asks for it and a post without it is refused.
func TestAnsweringOrCancellingAJobNeedsThePINStepUp(t *testing.T) {
	s, now := pinServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	c.change("/unlock", url.Values{"pin": {testcred.Secret("owner")}})

	id := asking(t, c)
	*now = now.Add(stepUpWindow + time.Second)
	page := c.get("/jobs/" + id).Body.String()
	if strings.Count(page, `name="pin"`) < 2 {
		t.Errorf("the page asks for no PIN on answer and cancel once the window has passed:\n%.600s", page)
	}
	for _, path := range []string{"/answer", "/cancel"} {
		if rec := c.change("/jobs/"+id+path, url.Values{"answer": {"yes"}}); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "PIN needed") {
			t.Errorf("%s without the PIN: %d", path, rec.Code)
		}
		if rec := c.change("/jobs/"+id+path, url.Values{"answer": {"yes"}, "pin": {testcred.Secret("wrong")}}); rec.Code != http.StatusForbidden {
			t.Errorf("%s with a wrong PIN: %d", path, rec.Code)
		}
	}
	if v, _ := s.jobs.Get(id).View(); v.State != jobs.Waiting {
		t.Fatalf("a refused post moved the job on: %s", v.State)
	}
	if rec := c.change("/jobs/"+id+"/cancel", url.Values{"pin": {testcred.Secret("owner")}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("cancel with the PIN: %d\n%.300s", rec.Code, rec.Body.String())
	}
	if v := waitJob(t, s, id); v.State != jobs.Cancelled {
		t.Errorf("job = %s, want cancelled", v.State)
	}
}
