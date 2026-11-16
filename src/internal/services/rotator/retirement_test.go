package rotator

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/reconciler"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// A retirement record that cannot be kept is said, in the rotation's own
// summary, rather than dropped: retired.json is what tells the person where
// the outgoing key still logs in (audit B-05).
func TestARetirementRecordThatCannotBeKeptIsReported(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &m); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	inv := inventory.New()
	if _, err := reconciler.New(p, &m, inv, false).Reconcile(false, ""); err != nil {
		t.Fatal(err)
	}
	keyName, _ := m.ResolvedKeyName("vcs", m.Profiles["vcs"].Hosts[0])
	retired := inventory.RetiredPath(p.Inventory())
	if err := os.MkdirAll(filepath.Dir(retired), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retired, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	rep, err := New(p, &m, inv).Rotate(keyName, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Committed {
		t.Fatalf("rotate not committed: %+v", rep)
	}
	if len(rep.Warnings) != 1 || !strings.Contains(rep.Warnings[0], "retired.json") {
		t.Fatalf("warnings = %q, want one naming retired.json", rep.Warnings)
	}
	if !strings.Contains(rep.Format(), "WARNING: "+rep.Warnings[0]) {
		t.Errorf("the summary does not carry the warning:\n%s", rep.Format())
	}
	if b, _ := os.ReadFile(retired); string(b) != "{not json" {
		t.Errorf("a retired.json that could not be read was overwritten: %q", b)
	}
}
