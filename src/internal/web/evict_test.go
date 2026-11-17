package web

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/jobs"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/rotator"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// The previous-key switch is on the review with or without Force: off by
// default, on by default under Force and saying why, and turning it off
// under Force sticks (a 0 is sent after it).
func TestTheReviewOffersToTakeThePreviousKeyOffServers(t *testing.T) {
	s, p, _ := newTestServer(t)
	forcedHome(t, p)
	c := signIn(t, s)
	sw := regexpSwitch("evict_old")
	plain := c.get("/jobs/review?action=rotate-keys&all=1").Body.String()
	if m := sw.FindString(plain); m == "" || strings.Contains(m, "checked") {
		t.Errorf("without Force the switch is missing or on: %q", m)
	}
	if !strings.Contains(plain, "Take the previous key off servers") || !strings.Contains(plain, "kept in <code>old/</code>") {
		t.Error("the review does not offer the switch, or say the previous key is kept")
	}
	forced := c.get("/jobs/review?action=rotate-keys&all=1&force=1").Body.String()
	if m := sw.FindString(forced); !strings.Contains(m, "checked") || !strings.Contains(m, `value="auto"`) {
		t.Errorf("under Force the switch is not on by default: %q", m)
	}
	for _, want := range []string{`name="evict_old" value="0"`, "On by default with Force", "taken off servers, then dropped", "--evict-old"} {
		if !strings.Contains(forced, want) {
			t.Errorf("the forced review lacks %q", want)
		}
	}
	kept := c.get("/jobs/review?action=rotate-keys&all=1&force=1&evict_old=0").Body.String()
	if m := sw.FindString(kept); strings.Contains(m, "checked") {
		t.Errorf("turned off under Force, the switch came back on: %q", m)
	}
	if !strings.Contains(kept, "--keep-old") {
		t.Error("a forced review keeping the previous key does not say --keep-old")
	}
}

// The choice is carried to the command line, which the job page reads back
// as badges.
func TestThePreviousKeyPolicyIsCarriedToTheCommandAndBadges(t *testing.T) {
	_, p, _ := newTestServer(t)
	forcedHome(t, p)
	for _, tc := range []struct {
		form       url.Values
		want, not  string
		evictsStep bool
	}{
		{url.Values{"force": {"1"}}, "--evict-old", "--keep-old", true},
		{url.Values{"force": {"1"}, "evict_old": {"auto"}}, "--evict-old", "--keep-old", true},
		{url.Values{"force": {"1"}, "evict_old": {"0"}}, "--keep-old", "--evict-old", false},
		{url.Values{"evict_old": {"1", "0"}}, "--evict-old", "--keep-old", true},
		{url.Values{"evict_old": {"auto"}}, "", "--evict-old", false},
		{url.Values{}, "", "--evict-old", false},
	} {
		f := tc.form
		f.Set("action", "rotate-keys")
		f.Set("all", "1")
		spec, err := buildJob(f, home{p: p})
		if err != nil {
			t.Fatal(err)
		}
		if (tc.want != "" && !slices.Contains(spec.scope.Args, tc.want)) || slices.Contains(spec.scope.Args, tc.not) {
			t.Errorf("%v: args %v, want %q and not %q", tc.form, spec.scope.Args, tc.want, tc.not)
		}
		if got := strings.Contains(strings.Join(spec.what, "\n"), "evicts the previous key"); got != tc.evictsStep {
			t.Errorf("%v: the steps say it evicts = %v", tc.form, got)
		}
		carried := carriedJob(f)
		if n := strings.Count(formString(carried), "evict_old="); n > 1 {
			t.Errorf("%v: the Start form carries %d evict_old values", tc.form, n)
		}
	}
	labels := func(args ...string) string {
		var out []string
		for _, b := range jobBadges("rotate", args) {
			out = append(out, b.Label)
		}
		return strings.Join(out, "|")
	}
	if g := labels("--all", "--force", "--evict-old"); !strings.Contains(g, "Take the previous key off servers") {
		t.Errorf("an evicting job's badges = %q", g)
	}
	if g := labels("--all", "--force", "--keep-old"); !strings.Contains(g, "Keep the previous key in old/") {
		t.Errorf("a keeping forced job's badges = %q", g)
	}
}

// regexpSwitch matches a review's switch named name.
func regexpSwitch(name string) *regexp.Regexp {
	return regexp.MustCompile(`<input class="form-check-input" type="checkbox" name="` + name + `"[^>]*>`)
}

func formString(fields []formField) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f.Name + "=" + f.Value + "&")
	}
	return b.String()
}

// liveOld is forcedHome after a forced job, with panel-a's previous key
// recorded as still logging in on panel-a.
func liveOld(t *testing.T, s *Server, p paths.Paths) {
	t.Helper()
	run := func(o rotateOpts) {
		j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Args: []string{"--all", "--force"}, Source: history.SourceWeb},
			func(j *jobs.Job) error { return rotateKeysJob(j, p, 3, keysvc.Selection{All: true}, o) })
		if err != nil {
			t.Fatal(err)
		}
		jv, _ := j.View()
		if v := waitJob(t, s, jv.ID); v.State != jobs.Done {
			t.Fatalf("the first forced job: %s %q", v.State, v.Err)
		}
	}
	run(rotateOpts{skip: true, force: true, evict: true})
	dir := filepath.Join(p.SSHDir, "profiles", "work")
	fp, err := keystore.New().Fingerprint(filepath.Join(dir, "old", "work_panel-a-ed25519.pub"))
	if err != nil {
		t.Fatal(err)
	}
	if err := (inventory.Retirements{fp: {Key: "work/work_panel-a-ed25519", NotRemoved: []string{"panel-a"}}}).
		Save(inventory.RetiredPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
}

// A forced job whose key's previous key still logs in evicts it by default
// and passes; told to keep it, that key fails and the job says why; and a
// forced dry run previews the eviction instead of failing the key.
func TestAForcedJobEvictsThePreviousKeyUnlessToldToKeepIt(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	forcedHome(t, p)
	liveOld(t, s, p)
	pub := func() string {
		b, _ := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", "work_panel-a-ed25519.pub"))
		return string(b)
	}
	start := func(args []string, run func(j *jobs.Job) error) jobs.View {
		j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Args: args, Source: history.SourceWeb}, run)
		if err != nil {
			t.Fatal(err)
		}
		jv, _ := j.View()
		return waitJob(t, s, jv.ID)
	}

	check := start([]string{"--all", "--check", "--force", "--evict-old"}, func(j *jobs.Job) error {
		return rotateCheckJob(j, p, forcedKeys, rotateOpts{force: true, evict: true})
	})
	sum, _ := check.Summary.(rotator.Summary)
	if check.State != jobs.Done || !strings.Contains(sum.Format(), "would evict old") {
		t.Errorf("the forced dry run: %s %q\n%s", check.State, check.Err, sum.Format())
	}
	keepCheck := start([]string{"--all", "--check", "--force", "--keep-old"}, func(j *jobs.Job) error {
		return rotateCheckJob(j, p, forcedKeys, rotateOpts{force: true})
	})
	if keepCheck.State != jobs.Failed || !strings.Contains(keepCheck.Err, "work_panel-a-ed25519") {
		t.Errorf("a forced dry run keeping a live previous key: %s %q", keepCheck.State, keepCheck.Err)
	}

	before := pub()
	kept := start([]string{"--all", "--force", "--keep-old"}, func(j *jobs.Job) error {
		return rotateKeysJob(j, p, 3, keysvc.Selection{All: true}, rotateOpts{skip: true, force: true})
	})
	if kept.State != jobs.Failed || pub() != before || !strings.Contains(strings.Join(kept.Lines, "\n"), "--evict-old") {
		t.Errorf("a forced job keeping a live previous key: %s %q", kept.State, kept.Err)
	}
	evicted := start([]string{"--all", "--force", "--evict-old"}, func(j *jobs.Job) error {
		return rotateKeysJob(j, p, 3, keysvc.Selection{All: true}, rotateOpts{skip: true, force: true, evict: true})
	})
	if evicted.State != jobs.Done || pub() == before {
		t.Fatalf("a forced job evicting: %s %q", evicted.State, evicted.Err)
	}
	if !strings.Contains(strings.Join(evicted.Lines, "\n"), "could not be taken off panel-a") {
		t.Errorf("the job does not say where the previous key still logs in:\n%s", strings.Join(evicted.Lines, "\n"))
	}
}
