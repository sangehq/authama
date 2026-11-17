package importer

import (
	"slices"
	"strings"
	"testing"
)

// Import says what it moves out of the user's config and what it does not
// take: the hand-written block it imported leaves the config (ssh reads one
// definition of the host), and Match and wildcard Host blocks, which used to
// be dropped without a word, are named and stay (review R-BUG-09).
func TestImportSaysWhatItLeavesAndWhatItSkips(t *testing.T) {
	im, _, cfgPath := setupImport(t, `Host *
    ServerAliveInterval 30

Host legacy
    HostName legacy.example
    User ops

Match host *.corp
    User corpuser

Host *.internal
    ProxyJump bastion
`)
	res, err := im.Run(cfgPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(res.Moved, "legacy") {
		t.Errorf("Moved = %v, want legacy", res.Moved)
	}
	if len(res.LeftInConfig) != 0 {
		t.Errorf("LeftInConfig = %v, want nothing left defined twice", res.LeftInConfig)
	}
	for _, want := range []string{"Match host *.corp", "Host *.internal"} {
		if !slices.Contains(res.NotImported, want) {
			t.Errorf("NotImported = %v, want %q", res.NotImported, want)
		}
	}
	out := res.Format()
	for _, want := range []string{"would move", "legacy", "Match host *.corp", "Host *.internal"} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary does not mention %q:\n%s", want, out)
		}
	}
}
