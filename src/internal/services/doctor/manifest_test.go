package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// A manifest that every other command refuses is a blocking finding: doctor
// used to load it with the error dropped, skip the drift checks, and say
// "clean" with ok:true (review R-BUG-03).
func TestAnUnreadableManifestFailsTheVerdict(t *testing.T) {
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(p.ConfigDir, "manifest.json"), []byte(`{"version":1, "profiles": {`), 0o600); err != nil {
		t.Fatal(err)
	}
	rep := New(p, nil, false).Run(false)
	if rep.ManifestError == "" {
		t.Fatal("a truncated manifest was not reported")
	}
	if rep.OK() {
		t.Error("doctor called a home with a truncated manifest healthy")
	}
	if !strings.Contains(rep.Format(), "manifest") || !strings.Contains(rep.Format(), rep.ManifestError) {
		t.Errorf("the report does not show the manifest error:\n%s", rep.Format())
	}
	b, err := rep.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var j struct {
		OK            bool   `json:"ok"`
		ManifestError string `json:"manifest_error"`
	}
	if err := json.Unmarshal(b, &j); err != nil || j.OK || j.ManifestError == "" {
		t.Errorf("--json: ok=%v manifest_error=%q (%v)", j.OK, j.ManifestError, err)
	}
}

// A home with no manifest yet is not that finding: init has not run.
func TestAMissingManifestIsNotAManifestError(t *testing.T) {
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	if rep := New(p, nil, false).Run(false); rep.ManifestError != "" {
		t.Errorf("a missing manifest was reported as broken: %q", rep.ManifestError)
	}
}
