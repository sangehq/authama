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

// The archive keeps one predecessor. A second rotation used to evict the
// first one's key from old/ even while a host it was never removed from still
// authorised it, so that login could no longer be rolled back to or cleaned up
// with the key in hand (review R-BUG-12). The rotation now stops and names the
// hosts, unless told to evict.
func TestASecondRotationKeepsAPredecessorThatStillLogsIn(t *testing.T) {
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
	oldSlot := filepath.Join(p.SSHDir, "profiles", "vcs", "old", keyName)

	// The first rotation leaves the outgoing key on the panel (a by-hand target).
	if rep, err := New(p, &m, inv).Rotate(keyName, true, ""); err != nil || !rep.Committed {
		t.Fatalf("first rotation: %+v %v", rep, err)
	}
	first, err := os.ReadFile(oldSlot)
	if err != nil {
		t.Fatal(err)
	}

	rep, err := New(p, &m, inv).Rotate(keyName, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Committed {
		t.Fatal("the second rotation evicted a predecessor that still logs in")
	}
	if !strings.Contains(rep.Message, "panel") {
		t.Errorf("the refusal does not name the host: %q", rep.Message)
	}
	if got, _ := os.ReadFile(oldSlot); string(got) != string(first) {
		t.Error("old/ changed although the rotation stopped")
	}

	// Told to, it evicts.
	if rep, err := New(p, &m, inv).EvictPredecessor().Rotate(keyName, true, ""); err != nil || !rep.Committed {
		t.Fatalf("rotation told to evict: %+v %v", rep, err)
	}
}
