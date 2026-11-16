package web

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/jobs"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/rotator"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// A dry run chosen with Force keeps it: the check job tests what the forced
// rotation would do, and its command names every option chosen.
func TestADryRunKeepsForceAndEveryOption(t *testing.T) {
	_, p, _ := newTestServer(t)
	for _, f := range []url.Values{
		{"action": {"rotate-keys"}, "all": {"1"}, "check": {"1"}, "force": {"1"}, "allow_unverified": {"1"}, "no_backup": {"1"}},
		{"action": {"rotate"}, "key": {"work/work_gh-ed25519"}, "check": {"1"}, "force": {"1"}, "allow_unverified": {"1"}, "no_backup": {"1"}},
	} {
		spec, err := buildJob(f, home{p: p})
		if err != nil {
			t.Fatal(err)
		}
		describeJob(f, &spec)
		if !spec.check {
			t.Errorf("%s: not a dry run", f.Get("action"))
		}
		for _, flag := range []string{"--check", "--force", "--allow-unverified", "--no-key-backup"} {
			if !slices.Contains(spec.scope.Args, flag) {
				t.Errorf("%s: the dry run's command %v lacks %s", f.Get("action"), spec.scope.Args, flag)
			}
		}
		if !strings.Contains(spec.title, "forced") || !strings.HasPrefix(spec.title, "Dry run:") {
			t.Errorf("%s: title %q does not say it is a forced dry run", f.Get("action"), spec.title)
		}
	}
}

// forcedHome is a profile whose two keys go to targets that take the key by
// hand (ploi), and a declared key no host uses, each with a key on disk.
func forcedHome(t *testing.T, p paths.Paths) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","keys":[{"name":"work_spare-ed25519"}],"hosts":[
	    {"alias":"panel-a","hostname":"ploi-a.example","user":"x","provider":"ploi"},
	    {"alias":"panel-b","hostname":"ploi-b.example","user":"x","provider":"ploi"}]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(p.SSHDir, "profiles", "work")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, n := range forcedKeys {
		path := filepath.Join(dir, strings.TrimPrefix(n, "work/"))
		_ = os.Remove(path)
		_ = os.Remove(path + ".pub")
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "test", "-f", path).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v\n%s", err, out)
		}
	}
}

var forcedKeys = []string{"work/work_panel-a-ed25519", "work/work_panel-b-ed25519", "work/work_spare-ed25519"}

// Forced, a dry run passes when every key would be rotated or replaced: the
// paste targets and the key no host uses read "would replace here".
func TestAForcedDryRunJobPassesAndSaysWhatItWouldReplace(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	forcedHome(t, p)
	names, err := selected(p, keysvc.Selection{All: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Fatalf("a forced selection = %v, want the key no host uses too", names)
	}
	if plain, _ := selected(p, keysvc.Selection{All: true}, false); len(plain) != 2 {
		t.Errorf("an unforced selection = %v, want only the keys a host uses", plain)
	}
	j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Args: []string{"--all", "--check", "--force"}, Source: history.SourceWeb, DryRun: true},
		func(j *jobs.Job) error { return rotateCheckJob(j, p, names, true) })
	if err != nil {
		t.Fatal(err)
	}
	jv, _ := j.View()
	v := waitJob(t, s, jv.ID)
	if v.State != jobs.Done {
		t.Fatalf("job = %s %q, want it done", v.State, v.Err)
	}
	sum, _ := v.Summary.(rotator.Summary)
	if got := sum.Totals(); got != "3 would replace here" {
		t.Errorf("totals = %q", got)
	}
}

// Forced, the real job replaces every key chosen, never stopping to ask for
// a paste, and passes while listing what is left to deploy.
func TestAForcedRotateJobReplacesEveryKeyWithoutAskingForAPaste(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	forcedHome(t, p)
	before := map[string]string{}
	pub := func(n string) string {
		b, _ := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", strings.TrimPrefix(n, "work/")+".pub"))
		return string(b)
	}
	for _, n := range forcedKeys {
		before[n] = pub(n)
	}
	j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Args: []string{"--all", "--force"}, Source: history.SourceWeb},
		func(j *jobs.Job) error { return rotateKeysJob(j, p, 3, keysvc.Selection{All: true}, false, true, true) })
	if err != nil {
		t.Fatal(err)
	}
	jv, _ := j.View()
	v := waitJob(t, s, jv.ID)
	if v.State != jobs.Done {
		t.Fatalf("job = %s %q, want it done", v.State, v.Err)
	}
	for _, n := range forcedKeys {
		if pub(n) == before[n] {
			t.Errorf("%s was not replaced", n)
		}
	}
	sum, _ := v.Summary.(rotator.Summary)
	if got := sum.Totals(); got != "3 replaced" || len(sum.Needs()) == 0 {
		t.Errorf("totals = %q, needs = %+v", got, sum.Needs())
	}
}

// A job's page shows every option it was started with, as the review labels
// them, under its title, and its command line carries every flag.
func TestAJobPageShowsEveryOptionChosen(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Args: []string{"--all", "--check", "--force", "--allow-unverified", "--no-key-backup"},
		Source: history.SourceWeb, DryRun: true}, func(j *jobs.Job) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	j.SetTitle("Dry run: rotate 10 keys (forced)")
	jv, _ := j.View()
	waitJob(t, s, jv.ID)
	page := c.get("/jobs/" + jv.ID).Body.String()
	for _, want := range []string{`id="job-chosen"`, "Dry run (check)", "Force: replace what cannot rotate in place",
		"Go on when a login cannot be verified", "Skip the encrypted backup", "Scope: every key", "badge bg-red-lt",
		"--check --force --allow-unverified --no-key-backup"} {
		if !strings.Contains(page, want) {
			t.Errorf("the job page lacks %q", want)
		}
	}
}

// The options read back from a job's command: a profile, keys, and the
// flags of the other key jobs.
func TestJobOptionsReadTheCommand(t *testing.T) {
	got := func(cmd string, args ...string) string {
		var out []string
		for _, b := range jobBadges(cmd, args) {
			out = append(out, b.Label)
		}
		return strings.Join(out, "|")
	}
	if g := got("rotate", "work", "--yes"); g != "Scope: profile work" {
		t.Errorf("a profile's rotation = %q", g)
	}
	if g := got("rotate", "work/a", "work/b", "--yes"); g != "Scope: 2 keys" {
		t.Errorf("two keys = %q", g)
	}
	if g := got("rotate", "work/a", "--force", "--yes"); g != "Force: replace what cannot rotate in place|Scope: key work/a" {
		t.Errorf("one key, forced = %q", g)
	}
	if g := got("key delete", "work/a", "--revoke", "--purge", "--yes"); !strings.Contains(g, "Revoke each from its targets first") ||
		!strings.Contains(g, "Delete the key files too") {
		t.Errorf("delete's options = %q", g)
	}
	if g := got("deploy", "work/a", "--yes"); g != "" {
		t.Errorf("deploy has no options, got %q", g)
	}
}
