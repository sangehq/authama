package importer

import (
	"slices"
	"strings"
	"testing"
)

// Import says what it leaves in the user's config and what it does not take:
// the hand-written blocks it copied stay where they are (ssh reads them first,
// so doctor reports them shadowed until they go), and Match and wildcard Host
// blocks were dropped without a word (review R-BUG-09).
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
	if !slices.Contains(res.LeftInConfig, "legacy") {
		t.Errorf("LeftInConfig = %v, want legacy", res.LeftInConfig)
	}
	for _, want := range []string{"Match host *.corp", "Host *.internal"} {
		if !slices.Contains(res.NotImported, want) {
			t.Errorf("NotImported = %v, want %q", res.NotImported, want)
		}
	}
	out := res.Format()
	for _, want := range []string{"legacy", "Match host *.corp", "Host *.internal", "doctor"} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary does not mention %q:\n%s", want, out)
		}
	}
}
