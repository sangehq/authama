package cli

import (
	"os"
	"strings"
	"testing"
)

// doctor exits non-zero on a manifest every other command refuses; it used to
// print "doctor: clean" and exit 0 (review R-BUG-03).
func TestDoctorFailsOnAnUnreadableManifest(t *testing.T) {
	p := editHome(t)
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1, "profiles": {`), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runErr(t, "doctor")
	if err == nil {
		t.Fatalf("doctor exited 0 on a truncated manifest:\n%s", out)
	}
	if !strings.Contains(out, "FAIL manifest") {
		t.Errorf("doctor did not say the manifest is broken:\n%s", out)
	}
}
