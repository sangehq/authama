package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/mutation"
)

// Deleting by profile deletes each of its keys no host uses, names the ones a
// host uses, keeps them, and says it did not finish cleanly.
func TestKeyDeleteTakesAProfileAndKeepsKeysAHostUses(t *testing.T) {
	p := promptHome(t)
	t.Cleanup(mutation.Release)
	out, err := runErr(t, "ssh", "key", "delete", "work", "--yes")
	if !errors.Is(err, errNotClean) {
		t.Fatalf("want errNotClean (a key a host uses was left), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "left alone, a host uses them") || !strings.Contains(out, "work/work_gh-ed25519") {
		t.Fatalf("the kept keys are not named:\n%s", out)
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	refs, _ := m.KeyRefs()
	for _, r := range refs {
		if r.KeyName == "spare" {
			t.Fatalf("work/spare is still declared: %v", refs)
		}
	}
	// A single key keeps its own path and wording.
	if _, err := runErr(t, "ssh", "key", "delete", "work/work_gh-ed25519", "--yes"); err == nil {
		t.Fatal("a key a host uses was deleted on its own")
	}
	// Nothing named is an error, not an empty batch.
	if _, err := runErr(t, "ssh", "key", "delete", "--yes"); err == nil || !strings.Contains(err.Error(), "name keys") {
		t.Fatalf("no selection: %v", err)
	}
}
