package web

import (
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/services/mutation"
)

// A job that counts its steps streams them as "progress" events, and its page
// draws a bar with the count and percentage, so a long rotate or revoke says
// how far it has got. A job that counts nothing shows a moving bar and says
// it gives no percentage, rather than inventing one.
func TestAJobPageShowsItsProgress(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	j, err := s.jobs.Start(p, mutation.Scope{Command: "key revoke", Source: history.SourceWeb}, func(j *jobs.Job) error {
		j.Progress(1, 4, "revoking work/gh")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	waitJob(t, s, v.ID)
	page := c.get("/jobs/" + v.ID).Body.String()
	for _, want := range []string{"data-job-progress", `max="4" value="1"`, "1 of 4 · 25%", "revoking work/gh"} {
		if !strings.Contains(page, want) {
			t.Errorf("the job page lacks %q", want)
		}
	}
	if ev := c.get("/jobs/" + v.ID + "/events").Body.String(); !strings.Contains(ev, "event: progress\ndata: 1 4 revoking work/gh\n\n") {
		t.Errorf("the stream carries no progress event:\n%s", ev)
	}

	quiet, err := s.jobs.Start(p, mutation.Scope{Command: "deploy", Source: history.SourceWeb}, func(j *jobs.Job) error {
		_, _ = j.Write([]byte("one step\n"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	qv, _ := quiet.View()
	waitJob(t, s, qv.ID)
	if page := c.get("/jobs/" + qv.ID).Body.String(); strings.Contains(page, "data-job-progress") {
		t.Errorf("a finished job that counted nothing still shows a progress card")
	}
}

// The Overview walks a new home through its first steps, ticking each one the
// home has done, and a read-only server, which can do none of them, shows no
// such card.
func TestTheOverviewOffersGetStartedUntilEveryStepIsDone(t *testing.T) {
	steps := setupFor(dashboard{})
	if steps.Done != 0 || steps.Complete() || len(steps.Steps) < 4 {
		t.Fatalf("an empty home's setup = %+v", steps)
	}
	var full dashboard
	full.Profiles, full.Hosts, full.KeysOnDisk, full.Tokens = 1, 1, 1, 1
	if got := setupFor(full); !got.Complete() || got.Percent() != 100 {
		t.Errorf("a home with every step done: %+v", got)
	}

	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/").Body.String()
	if !strings.Contains(body, "data-setup-card") {
		// The fixture home has no token, so at least that step is open.
		t.Fatalf("the overview offers no Get started card:\n%s", body)
	}
	for _, href := range []string{"/tokens#add-token"} {
		if !strings.Contains(body, `href="`+href+`"`) {
			t.Errorf("the open step %s has no Start link", href)
		}
	}
}

// Every page carries the Actions menu, whose entries open the forms people
// come back for; a read-only server hides it, since it can do none of them.
func TestTheActionsMenuOpensTheKeyJobs(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/hosts").Body.String()
	for _, href := range []string{"/keys#rotate-keys", "/keys#revoke-keys", "/keys#delete-keys", "/hosts#knownhosts-init", "/tokens#add-token"} {
		if !strings.Contains(body, `href="`+href+`"`) {
			t.Errorf("the Actions menu lacks %s", href)
		}
	}
	keys := c.get("/keys").Body.String()
	for _, id := range []string{`id="rotate-keys"`, `id="revoke-keys"`, `id="delete-keys"`, `id="mint-keys"`} {
		if !strings.Contains(keys, id) {
			t.Errorf("the keys page has no panel %s for the menu to open", id)
		}
	}

	p, _ := fixtureHome(t)
	ro, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	roBody := signIn(t, ro).get("/").Body.String()
	if strings.Contains(roBody, "/keys#rotate-keys") || strings.Contains(roBody, "data-setup-card") {
		t.Error("a read-only server offers the Actions menu or the Get started card")
	}
}
