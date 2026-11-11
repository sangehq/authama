package web

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

// A job's review names the job, lists its checks, its steps and the keys it
// works through, and starts only from a button that asks to be confirmed,
// never one that spells out the command. Without a backup recipient a rotate
// cannot start, and the page says why.
func TestAJobReviewHasGuardrails(t *testing.T) {
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "")
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	key := fixtureKey(t, c)
	body := c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}}).Body.String()
	for _, want := range []string{"<h1 class=\"page-title\">Rotate " + key, "Before it starts", "A backup recipient is set",
		"What it does, in order", `data-confirm="m-job"`, `id="m-job"`, "Start: Rotate " + key} {
		if !strings.Contains(body, want) {
			t.Errorf("the review lacks %q", want)
		}
	}
	if !regexp.MustCompile(`data-confirm="m-job" disabled`).MatchString(body) {
		t.Error("with no backup recipient, the rotate can still be started")
	}
	if strings.Contains(body, ">Run authama") {
		t.Error("the start button spells out the command")
	}

	// With age on the PATH (a stand-in, so the test does not depend on the
	// machine having it) and a recipient set, every check passes.
	withStandInAge(t)
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs3290gq")
	body = c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}}).Body.String()
	if regexp.MustCompile(`data-confirm="m-job" disabled`).MatchString(body) {
		t.Error("with a backup recipient set, the rotate still cannot start")
	}
}

// A job that plans its items lists them on its page with their states, and
// streams each change as an "items" event.
func TestAJobPageListsItsItemsOneAtATime(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	j, err := s.jobs.Start(p, mutation.Scope{Command: "key revoke", Source: history.SourceWeb}, func(j *jobs.Job) error {
		j.Plan([]string{"work/a", "work/b"})
		j.Progress(1, 2, "revoking work/b")
		j.Failed("work/a")
		j.Progress(2, 2, "")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	waitJob(t, s, v.ID)
	page := c.get("/jobs/" + v.ID).Body.String()
	for _, want := range []string{`data-job-item="work/a"`, `job-run-item is-failed`, `data-job-item="work/b"`, `job-run-item is-done`} {
		if !strings.Contains(page, want) {
			t.Errorf("the job page lacks %q", want)
		}
	}
	if ev := c.get("/jobs/" + v.ID + "/events").Body.String(); !strings.Contains(ev, "event: items\ndata: work/a=failed work/b=done\n\n") {
		t.Errorf("the stream carries no items event:\n%s", ev)
	}
}

// withStandInAge puts an executable named age first on the PATH, so a check
// that looks for it passes on a machine without age installed (CI).
func withStandInAge(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"age", "age.bat"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Without age on the PATH a job that backs up first cannot start, and the
// check says how to install it.
func TestAJobThatBacksUpChecksAgeIsInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "age1qyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqszqgpqyqs3290gq")
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	body := c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {fixtureKey(t, c)}}).Body.String()
	if !strings.Contains(body, "age is installed") || !strings.Contains(body, "brew install age") {
		t.Error("the review does not check that age is installed")
	}
	if !regexp.MustCompile(`data-confirm="m-job" disabled`).MatchString(body) {
		t.Error("without age, the rotate can still be started")
	}
}
