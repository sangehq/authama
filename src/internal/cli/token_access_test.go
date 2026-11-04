package cli

import (
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// A secret that leaves its file is on the record: copying it, or printing it
// with --reveal, files an operation naming the token and its file, and no
// trace of the value. A change to a token is found by the same name.
func TestReadingATokenIsRecordedWithoutItsValue(t *testing.T) {
	b := tokenSandbox(t)
	p, err := paths.ResolveDev(nil, "", b.root)
	if err != nil {
		t.Fatal(err)
	}
	clipboardWrite = func(string) error { return nil }
	b.ok(t, credGhpCopyme+"\n", "set", "--service", "github", "--server", "github.com")
	b.ok(t, "", "copy", "composer", "github.com")
	b.ok(t, "", "show", "composer", "github.com", "--reveal", "--yes")

	ops := historyOps(t, p, history.Filter{Token: "composer/github.com"})
	want := map[string]string{"token set": "", "token copy": "token.copied", "token show": "token.revealed"}
	for _, o := range ops {
		ev, ok := want[o.Command]
		if !ok {
			continue
		}
		delete(want, o.Command)
		if ev == "" {
			continue
		}
		var names []string
		for _, e := range o.Events {
			names = append(names, e.Event)
			if strings.Contains(string(e.Fields), credGhpCopyme) {
				t.Errorf("%s recorded the secret: %s", o.Command, e.Fields)
			}
		}
		if !strings.Contains(strings.Join(names, " "), ev) {
			t.Errorf("%s events = %v, want %s", o.Command, names, ev)
		}
	}
	if len(want) != 0 {
		t.Errorf("history --token composer/github.com is missing %v (found %d ops)", want, len(ops))
	}
	if got := historyOps(t, p, history.Filter{Token: "composer"}); len(got) != len(ops) {
		t.Errorf("a bare tool found %d operations, the full name %d", len(got), len(ops))
	}
	if got := historyOps(t, p, history.Filter{Token: "npm"}); len(got) != 0 {
		t.Errorf("--token npm found %d composer operations", len(got))
	}
	// A plain show reads no secret and records nothing.
	before := len(historyOps(t, p, history.Filter{}))
	b.ok(t, "", "show", "composer", "github.com")
	if after := len(historyOps(t, p, history.Filter{})); after != before {
		t.Errorf("a masked show recorded %d operation(s)", after-before)
	}
}
