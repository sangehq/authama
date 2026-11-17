package jobs

import (
	"testing"

	"github.com/sangehq/authama/src/internal/services/mutation"
)

// A job belongs to the owner it was started for, and to nobody else; a job
// started with no owner belongs to nobody, and the empty owner owns nothing
// (review R-GAP-06).
func TestAJobBelongsToTheOwnerItWasStartedFor(t *testing.T) {
	p := home(t)
	r := NewRegistry()
	j, err := r.StartFor("session-a", p, mutation.Scope{Command: "rotate"}, func(j *Job) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	until(t, j, Done)
	if !j.OwnedBy("session-a") || j.OwnedBy("session-b") || j.OwnedBy("") {
		t.Errorf("owned by a %v, by b %v, by nobody %v", j.OwnedBy("session-a"), j.OwnedBy("session-b"), j.OwnedBy(""))
	}
	k, err := r.Start(p, mutation.Scope{Command: "rotate"}, func(j *Job) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	until(t, k, Done)
	if k.OwnedBy("session-a") || k.OwnedBy("") {
		t.Error("a job started with no owner is owned by someone")
	}
}
