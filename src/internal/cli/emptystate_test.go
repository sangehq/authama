package cli

import (
	"strings"
	"testing"
)

// A list that is empty because nothing exists yet says how the first one comes
// to be, as the web UI's empty states do. The old phrase stays at the start of
// each line, so a script that matched it still does.
func TestEmptyListsTeachTheFirstStep(t *testing.T) {
	editHome(t)
	t.Setenv("AUTHAMA_VM_HYPERVISORS", "")
	cases := []struct {
		args    []string
		phrase  string
		teaches string
	}{
		{[]string{"ssh", "snapshots", "list"}, "no snapshots yet", "the first command that changes ~/.ssh takes one"},
		{[]string{"ssh", "validate"}, "no managed keys to validate", "ssh key add"},
		{[]string{"ssh", "vm", "sync", "--dry-run"}, "no VM hosts in the manifest", "ssh vm add <profile> <alias>"},
	}
	for _, tc := range cases {
		out := run(t, tc.args...)
		if !strings.HasPrefix(strings.TrimSpace(out), tc.phrase) {
			t.Errorf("%v: does not start with %q:\n%s", tc.args, tc.phrase, out)
		}
		if !strings.Contains(out, tc.teaches) {
			t.Errorf("%v: does not say %q:\n%s", tc.args, tc.teaches, out)
		}
	}
}
