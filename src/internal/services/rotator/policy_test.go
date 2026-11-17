package rotator

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/reconciler"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// liveOldHome is a key rotated once onto a panel (a by-hand target), so its
// previous key in old/ is recorded as still logging in there.
func liveOldHome(t *testing.T) (paths.Paths, *manifest.Manifest, *inventory.Inventory, string) {
	t.Helper()
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
	if rep, err := New(p, &m, inv).Rotate(keyName, true, ""); err != nil || !rep.Committed {
		t.Fatalf("first rotation: %+v %v", rep, err)
	}
	return p, &m, inv, keyName
}

// The dry run sees the previous key: kept while it still logs in, the key
// would fail, and a forced dry run does not hide that (--force --keep-old
// stops the same way); evicted, it passes and says where it would come off.
func TestADryRunPreviewsThePreviousKeyPolicy(t *testing.T) {
	p, m, inv, keyName := liveOldHome(t)

	kept, err := New(p, m, inv).Check(keyName)
	if err != nil {
		t.Fatal(err)
	}
	if kept.ProblemKind != KindOldLive || !strings.Contains(kept.Problem, "panel") {
		t.Fatalf("keeping a previous key that still logs in = %+v", kept)
	}
	if f := SummarizeForcedCheck([]CheckReport{kept}, 0).Failed(); len(f) != 1 {
		t.Errorf("a forced dry run that keeps the previous key passed it: %v", f)
	}

	r := New(p, m, inv)
	r.EvictOld = true
	ev, err := r.Check(keyName)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Problem != "" || !slices.Equal(ev.Evict, []string{"panel"}) {
		t.Fatalf("evicting = %+v", ev)
	}
	if !strings.Contains(ev.Format(), "would evict old from 1 host(s): panel") {
		t.Errorf("the dry run does not say it would evict:\n%s", ev.Format())
	}
	sum := SummarizeForcedCheck([]CheckReport{ev}, 0)
	if len(sum.Failed()) != 0 || !strings.Contains(sum.Keys[0].Note, "would evict old from 1 host(s) (panel)") {
		t.Errorf("the forced dry run that evicts:\n%s", sum.Format())
	}
}

// A rotation that evicts takes the previous key off what it can and names
// what it could not: a panel takes keys by hand, so the key still logs in
// there, and the report and the summary say so.
func TestARotationThatEvictsSaysWhereThePreviousKeyStillLogsIn(t *testing.T) {
	p, m, inv, keyName := liveOldHome(t)
	r := New(p, m, inv)
	r.EvictOld = true
	rep, err := r.Rotate(keyName, true, "")
	if err != nil || !rep.Committed {
		t.Fatalf("rotation told to evict: %+v %v", rep, err)
	}
	if !slices.Equal(rep.StillTrusted, []string{"panel"}) || !strings.Contains(rep.Format(), "could not be taken off panel") {
		t.Errorf("the report = %+v\n%s", rep, rep.Format())
	}
	ref := "vcs/" + keyName
	sum := SummarizeRun([]RunResult{{Ref: ref, Report: &rep}}, nil, 0)
	if k := sum.Keys[0]; k.Kind != KindNotRemoved || !strings.Contains(k.Note, "could not be taken off panel") {
		t.Errorf("the summary does not say where the previous key still logs in:\n%s", sum.Format())
	}
}
