package cli

import (
	"testing"
)

// settings set and unset are changes like any other: recorded in the
// history, which also means they ran under the lock that renews the owner
// seal. Before, they wrote authama.json outside it, so the next change
// warned that the file had changed behind authama's back.
func TestSettingsChangesAreRecordedLikeAnyOther(t *testing.T) {
	p := editHome(t)
	run(t, "settings", "set", "web.port", "8000")
	run(t, "settings", "unset", "web.port")
	var got []string
	for _, op := range liveOps(t, p) {
		got = append(got, op.Command)
	}
	want := map[string]bool{"settings set": false, "settings unset": false}
	for _, c := range got {
		if _, ok := want[c]; ok {
			want[c] = true
		}
	}
	for c, seen := range want {
		if !seen {
			t.Errorf("%q was not recorded; recorded %v", c, got)
		}
	}
}
