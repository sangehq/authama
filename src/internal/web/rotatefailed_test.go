package web

import (
	"os"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/jobs"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/mutation"
)

// The list "retry the failed ones" reads is kept by the job as the CLI keeps
// it: a write that fails fails the job, rather than leaving a stale list
// behind a run that looks finished (audit B-06).
func TestARotateJobThatCannotSaveTheFailedListSaysSo(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	// The state directory is a file, so the failed list cannot be written.
	if err := os.WriteFile(p.StateDir(), []byte("in the way\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sel := keysvc.Selection{Selectors: []string{"work"}}
	j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Source: history.SourceWeb}, func(j *jobs.Job) error {
		return rotateKeysJob(j, p, 3, sel, true, true, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	jv, _ := j.View()
	v := waitJob(t, s, jv.ID)
	if v.State != jobs.Failed || !strings.Contains(v.Err, "retry") {
		t.Errorf("job = %s %q, want it failed saying the retry list was not saved", v.State, v.Err)
	}
}

// rotate --check from the page keeps the same list, and says so too.
func TestARotateCheckJobThatCannotSaveTheFailedListSaysSo(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	if err := os.WriteFile(p.StateDir(), []byte("in the way\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j, err := s.jobs.Start(p, mutation.Scope{Command: "rotate", Source: history.SourceWeb, DryRun: true}, func(j *jobs.Job) error {
		return rotateCheckJob(j, p, []string{"work/no-such-key"})
	})
	if err != nil {
		t.Fatal(err)
	}
	jv, _ := j.View()
	v := waitJob(t, s, jv.ID)
	if v.State != jobs.Failed || !strings.Contains(v.Err, "retry") {
		t.Errorf("job = %s %q, want it failed saying the retry list was not saved", v.State, v.Err)
	}
}
