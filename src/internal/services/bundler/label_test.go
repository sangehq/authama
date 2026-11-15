package bundler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A key and its archived predecessor in old/ share a file name, and the
// restore summary listed both under that name with nothing to tell them
// apart (review R-INC-06). Each line now names the key's path.
func TestTheRestoreSummaryTellsAKeyFromItsOldCopy(t *testing.T) {
	src, cfg := writeSrc(t)
	old := filepath.Join(src, "profiles", "work", "old")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"work_a-ed25519": "PRIV-OLD\n", "work_a-ed25519.pub": "ssh-ed25519 BBBB old\n"} {
		if err := os.WriteFile(filepath.Join(old, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := New(src, cfg, fakeCipher{}).Bundle("age1recipient", t.TempDir(), "20260101-000000")
	if err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	rr, err := New(filepath.Join(base, ".ssh"), filepath.Join(base, "cfg"), fakeCipher{}).Restore(res.AgePath, "", fakeFP)
	if err != nil {
		t.Fatal(err)
	}
	out := rr.Format()
	for _, want := range []string{"profiles/work/work_a-ed25519 ", "profiles/work/old/work_a-ed25519 "} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary does not name %q:\n%s", strings.TrimSpace(want), out)
		}
	}
}
