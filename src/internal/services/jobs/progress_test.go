package jobs

import (
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/mutation"
)

// A job that counts its steps shows how far it has got; one that does not
// shows no progress at all rather than a made-up one.
func TestAJobReportsTheProgressItCounts(t *testing.T) {
	p := home(t)
	r := NewRegistry()
	j, err := r.Start(p, mutation.Scope{Command: "key revoke"}, func(j *Job) error {
		j.Progress(1, 4, "revoking work/gh")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	v := until(t, j, Done)
	if v.Progress == nil {
		t.Fatal("the job counted a step, and its view has no progress")
	}
	if v.Progress.Done != 1 || v.Progress.Total != 4 || v.Progress.Label != "revoking work/gh" || v.Progress.Percent() != 25 {
		t.Errorf("progress = %+v (%d%%)", *v.Progress, v.Progress.Percent())
	}

	quiet, err := r.Start(p, mutation.Scope{Command: "deploy"}, func(*Job) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if v := until(t, quiet, Done); v.Progress != nil {
		t.Errorf("a job that counts nothing shows progress %+v", *v.Progress)
	}
}

// The time left is the pace of the finished steps applied to the rest, and
// there is none to give before a step has finished or after the last.
func TestTimeLeftFollowsThePaceOfTheFinishedSteps(t *testing.T) {
	since := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	now := since.Add(30 * time.Second)
	for _, c := range []struct {
		name        string
		done, total int
		want        time.Duration
		ok          bool
	}{
		{"none finished yet", 0, 10, 0, false},
		{"three of ten in 30s", 3, 10, 70 * time.Second, true},
		{"all finished", 10, 10, 0, false},
		{"nothing counted", 0, 0, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Progress{Done: c.done, Total: c.total, Since: since}.Left(now)
			if ok != c.ok || got != c.want {
				t.Errorf("Left = %v, %v; want %v, %v", got, ok, c.want, c.ok)
			}
		})
	}
	if pct := (Progress{Done: 12, Total: 10}).Percent(); pct != 100 {
		t.Errorf("Percent past the total = %d, want it capped at 100", pct)
	}
}

// A job that plans its items shows each one's state: done before the one
// under way, running, waiting after it, and failed where it said so.
func TestPlannedItemsShowTheirState(t *testing.T) {
	p := home(t)
	r := NewRegistry()
	release := make(chan struct{})
	j, err := r.Start(p, mutation.Scope{Command: "rotate"}, func(j *Job) error {
		j.Plan([]string{"a", "b", "c", "d"})
		j.Progress(2, 4, "rotating c")
		j.Failed("a")
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var v View
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if v, _ = j.View(); v.Progress != nil && len(v.Progress.Items) == 4 && v.Progress.Items[0].State == ItemFailed {
			break
		}
	}
	close(release)
	var got []string
	if v.Progress != nil {
		for _, it := range v.Progress.Items {
			got = append(got, it.Name+"="+it.State)
		}
	}
	if want := "a=failed b=done c=running d=waiting"; strings.Join(got, " ") != want {
		t.Errorf("items = %q, want %q", strings.Join(got, " "), want)
	}
	until(t, j, Done)
}
