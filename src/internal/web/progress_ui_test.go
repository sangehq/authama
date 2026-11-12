package web

import (
	"github.com/simtabi/authama/src/internal/services/rotator"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

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
	release := make(chan struct{})
	j, err := s.jobs.Start(p, mutation.Scope{Command: "key revoke", Source: history.SourceWeb}, func(j *jobs.Job) error {
		j.Progress(1, 4, "revoking work/gh")
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	waitUntil(t, func() bool { v, _ := j.View(); return v.Progress != nil })
	page := c.get("/jobs/" + v.ID).Body.String()
	for _, want := range []string{"data-job-progress", `max="4" value="1"`, "1 of 4 · 25%", "revoking work/gh"} {
		if !strings.Contains(page, want) {
			t.Errorf("the running job's page lacks %q", want)
		}
	}
	close(release)
	waitJob(t, s, v.ID)
	if ev := c.get("/jobs/" + v.ID + "/events").Body.String(); !strings.Contains(ev, "event: progress\ndata: 1 4 revoking work/gh\n\n") {
		t.Errorf("the stream carries no progress event:\n%s", ev)
	}
	// Finished, the summary says how it went; the progress card goes.
	if page := c.get("/jobs/" + v.ID).Body.String(); strings.Contains(page, "data-job-progress") {
		t.Errorf("a finished job still shows its progress card")
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
	for _, want := range []string{"<h1 class=\"page-title mb-0\">Rotate " + key, "Before it starts", "A backup recipient is set",
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
	// The review is a page of its own: Cancel goes back to the key's page,
	// and its options review it again.
	for _, want := range []string{`href="/keys/` + key + `" class="btn btn-link`, `id="job-options"`, `name="force" value="1" data-autosubmit`} {
		if !strings.Contains(body, want) {
			t.Errorf("the review lacks %q", want)
		}
	}

	// Forced, it is marked dangerous everywhere: its title, a red Start, a
	// warning, the command and a confirm that names the lockout.
	body = c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}, "force": {"1"}}).Body.String()
	for _, want := range []string{"Rotate " + key + " (forced)", `class="btn btn-danger" data-confirm="m-job"`, "can lock you out until you deploy",
		"--force", "Yes, force it", `name="force" value="1" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("the forced review lacks %q", want)
		}
	}
	// A dry run is never forced, whatever the form says.
	body = c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}, "force": {"1"}, "check": {"1"}}).Body.String()
	if strings.Contains(body, "(forced)") || strings.Contains(body, "Yes, force it") {
		t.Error("a dry run is labelled forced")
	}
}

// A job that plans its items lists them on its page with their states, and
// streams each change as an "items" event.
func TestAJobPageListsItsItemsOneAtATime(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	release := make(chan struct{})
	j, err := s.jobs.Start(p, mutation.Scope{Command: "key revoke", Source: history.SourceWeb}, func(j *jobs.Job) error {
		j.Plan([]string{"work/a", "work/b"})
		j.Progress(1, 2, "revoking work/b")
		j.Failed("work/a")
		<-release
		j.Progress(2, 2, "")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	waitUntil(t, func() bool { v, _ := j.View(); return v.Progress != nil && v.Progress.Done == 1 })
	// Before the job sets a summary of its own, its page draws one that is
	// starting: the planned keys, waiting. Never the old console in its place.
	page := c.get("/jobs/" + v.ID).Body.String()
	for _, want := range []string{`id="job-summary"`, "<th scope=\"row\" colspan=\"4\"><span class=\"sum-branch\" aria-hidden=\"true\"></span>revoke", "waiting", `id="job-log"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the running job's page lacks %q", want)
		}
	}
	if strings.Contains(page, "job-run-item") || strings.Contains(page, `<h2 class="card-title">Output</h2>{{`) {
		t.Error("the job page still draws the old item list")
	}
	close(release)
	waitJob(t, s, v.ID)
	if ev := c.get("/jobs/" + v.ID + "/events").Body.String(); !strings.Contains(ev, "event: items\ndata: work/a=failed work/b=done\n\n") {
		t.Errorf("the stream carries no items event:\n%s", ev)
	}
}

// waitUntil polls cond until it holds, failing the test after a few seconds.
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	for range 200 {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the condition never held")
}

// A rotation's job page draws its summary: the keys grouped by owner and
// project, results coloured by how they went, each note once, what is left
// for you, and the command for the failed keys with a copy button.
func TestARotationJobPageDrawsItsSummary(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	never := "answers, but the current key does not log in: it was never deployed here"
	j, err := s.jobs.Start(p, mutation.Scope{Command: "ssh rotate", Args: []string{"--all", "--check"}, Source: history.SourceWeb}, func(j *jobs.Job) error {
		sum := rotator.SummarizeCheck([]rotator.CheckReport{
			{Key: "work/work_app-prd-web-ed25519", Hosts: []rotator.HostCheck{{Alias: "app-prd-web", Via: "ssh", Kind: rotator.KindNeverDeployed, Message: never}}},
			{Key: "work/work_app-prd-db-ed25519", Hosts: []rotator.HostCheck{{Alias: "app-prd-db", Via: "ssh", OK: true, Kind: rotator.KindLogsIn}}},
		}, time.Second)
		sum.Command = jobLine(j)
		j.SetSummary(sum)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	waitJob(t, s, v.ID)
	page := c.get("/jobs/" + v.ID).Body.String()
	for _, want := range []string{`id="job-summary"`, "$ </span>authama ssh rotate --all --check", `sum-badge sev-2">1 would fail`,
		`app<span class="sum-sep"> · </span><span class="sum-env">prd</span>`, "rotate</th>", "Full log", `id="job-log"`,
		`<tr class="sev-2">`, `href="#sum-note-1">[1]</a>`, `id="sum-note-1"`, never, "Needs you", "deploy first",
		"re-run failed:", `data-copy="authama ssh rotate --failed --check"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the summary lacks %q", want)
		}
	}
}

// A finished dry run with keys that would fail offers the next step: review
// those keys again, or with Force; and the stream says when its summary
// changed, so a page can draw it again.
func TestAFailedDryRunOffersToReviewAgainWithForce(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	j, err := s.jobs.Start(p, mutation.Scope{Command: "ssh rotate", Args: []string{"--all", "--check"}, Source: history.SourceWeb}, func(j *jobs.Job) error {
		j.SetSummary(rotator.Summary{Keys: rotator.Pending([]string{"work/work_a-ed25519"}, "testing")})
		j.SetSummary(rotator.SummarizeCheck([]rotator.CheckReport{
			{Key: "work/work_a-ed25519", Hosts: []rotator.HostCheck{{Alias: "a", Via: "ssh", Kind: rotator.KindUnreachable, Message: "a:22 unreachable"}}},
		}, time.Second))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v, _ := j.View()
	waitJob(t, s, v.ID)
	page := c.get("/jobs/" + v.ID).Body.String()
	for _, want := range []string{`id="job-next"`, "Review the failed keys again", "Review again with Force",
		`name="action" value="rotate-keys"`, `name="sel" value="work/work_a-ed25519"`, `name="force" value="1"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the dry run's page lacks %q", want)
		}
	}
	if ev := c.get("/jobs/" + v.ID + "/events").Body.String(); !strings.Contains(ev, "event: summary\ndata: 2\n\n") {
		t.Errorf("the stream carries no summary event:\n%s", ev)
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
