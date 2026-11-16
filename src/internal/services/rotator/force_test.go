package rotator

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// A forced dry run is a factory reset's preview: every key that would fail
// or pause is replaced on this computer instead, so none of them fails; each
// says why, and the summary counts them.
func TestAForcedDryRunReplacesWhatCannotRotateInPlace(t *testing.T) {
	reps := append(designRun(), CheckReport{Key: "spare/spare_unused-ed25519",
		Problem: "no host in the manifest uses key \"spare/spare_unused-ed25519\"", ProblemKind: KindNoHost})
	s := SummarizeForcedCheck(reps, time.Second)
	if f := s.Failed(); len(f) != 0 {
		t.Errorf("a forced dry run left keys failing: %v", f)
	}
	if got := s.Totals(); got != "7 would replace here · 4 would rotate" {
		t.Errorf("totals = %q", got)
	}
	out := s.Format()
	for _, want := range []string{"would replace here", "never deployed", "unreachable", "takes the new key by hand",
		"no host uses it", "Needs you", "deploy next"} {
		if !strings.Contains(out, want) {
			t.Errorf("the forced dry run does not say %q:\n%s", want, out)
		}
	}
	if s.Rerun() != "" {
		t.Errorf("a forced dry run with nothing failing offers a re-run: %q", s.Rerun())
	}
}

// A key that does not resolve cannot be replaced either: force does not hide it.
func TestAForcedDryRunStillFailsAKeyItCannotFind(t *testing.T) {
	s := SummarizeForcedCheck([]CheckReport{{Key: "work/nope", Problem: "no such key", ProblemKind: KindFailed}}, 0)
	if len(s.Failed()) != 1 {
		t.Errorf("an unknown key passed a forced dry run: %+v", s.Keys)
	}
}

// Without force a dry run is as it was.
func TestAPlainDryRunIsUnchangedByTheForcedOne(t *testing.T) {
	if got := SummarizeCheck(designRun(), 0).Totals(); got != "3 would fail · 3 would pause · 4 would rotate" {
		t.Errorf("totals = %q", got)
	}
}

// A key no host uses fails a rotation for that reason, and the summary names it.
func TestNoHostIsClassified(t *testing.T) {
	if k := Classify(`no host in the manifest uses key "work/x"`); k != KindNoHost {
		t.Errorf("Classify = %q, want %q", k, KindNoHost)
	}
	s := SummarizeRun([]RunResult{{Ref: "work/x", Err: errors.New(`no host in the manifest uses key "work/x"`)}}, []string{"work/x"}, 0)
	if s.Keys[0].Result != ResultReplaced || len(s.Failed()) != 0 {
		t.Errorf("a forced replace of an unused key = %+v", s.Keys[0])
	}
}
