package doctor

import (
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/preflight"
)

// --strict turns every warning into a failure, and the verdict says so rather
// than "clean". Without it a report with warnings passes, and says it has them.
func TestStrictFailsOnWarningsAndNeverSaysClean(t *testing.T) {
	clean := Report{ConfigInSync: true, Preflight: preflight.Report{RuntimeOK: true}}
	if !clean.OK() || !strings.Contains(clean.Format(), "doctor: clean") {
		t.Fatalf("a clean report is not clean:\n%s", clean.Format())
	}
	warn := clean
	warn.UnpinnedHosts = []string{"work/web1"}
	if !warn.OK() {
		t.Error("a warning failed the verdict without --strict")
	}
	if out := warn.Format(); strings.Contains(out, "doctor: clean") || !strings.Contains(out, "1 warning") {
		t.Errorf("a report with warnings says:\n%s", out)
	}
	warn.Strict = true
	if warn.OK() {
		t.Error("--strict passed a report with a warning")
	}
	if out := warn.Format(); strings.Contains(out, "clean") || !strings.Contains(out, "--strict") {
		t.Errorf("--strict verdict:\n%s", out)
	}
	strictClean := clean
	strictClean.Strict = true
	if !strictClean.OK() {
		t.Error("--strict failed a report with nothing to warn about")
	}
}
