package rotator

import (
	"strings"
	"testing"
)

// The summary names the operation and says plainly whether it happened: a
// failed rollback said "rotation ABORTED", and a forced rotation said
// ABORTED above the line saying the key was replaced (review R-INC-06).
func TestTheSummaryNamesTheOperationAndItsOutcome(t *testing.T) {
	cases := []struct {
		r            RotateReport
		want, reject string
	}{
		{RotateReport{KeyName: "k", Committed: true}, "rotated: k", "ABORTED"},
		{RotateReport{KeyName: "k"}, "rotation not done: k", "ABORTED"},
		{RotateReport{KeyName: "k", Op: OpRollback, Committed: true}, "rolled back: k", "rotat"},
		{RotateReport{KeyName: "k", Op: OpRollback}, "rollback not done: k", "rotat"},
	}
	for _, c := range cases {
		got := c.r.Format()
		if !strings.HasPrefix(got, c.want) || strings.Contains(strings.SplitN(got, "\n", 2)[0], c.reject) {
			t.Errorf("Format() = %q, want it to start %q and not say %q", got, c.want, c.reject)
		}
	}
}
