package web

import (
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/bundler"
	"github.com/simtabi/authama/src/internal/services/deployer"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/rotator"
)

// The jobs that work on one key, one bundle or a set of hosts draw the same
// summary as a rotation: a row per host or key, its result in words, and why.
func TestEveryJobDrawsItsRowsForTheSummary(t *testing.T) {
	cases := []struct {
		name string
		sum  rotator.Summary
		want []string
	}{
		{"deploy", rotator.Summary{Verb: "deploy", RerunCommand: "authama ssh deploy work/work_gh-ed25519", Keys: deployRows("work", "work_gh-ed25519", deployer.DeployReport{Records: []deployer.DeployRecord{
			{Target: "gh", Provider: "github", Verified: true},
			{Target: "box", Provider: "server", Error: true, Detail: "box:22 unreachable"},
			{Target: "panel", Provider: "ploi"},
		}})}, []string{"deploy\n", "× box", "not deployed", "host unreachable [1]", "! panel", "not verified [2]", "✓ gh", "check it logs in",
			"re-run failed: authama ssh deploy work/work_gh-ed25519"}},
		{"rollback", rotator.Summary{Verb: "rollback", Keys: rollbackRows("work", "work_gh-ed25519", rotator.RotateReport{Committed: true, Targets: []rotator.TargetResult{
			{Alias: "gh", Provider: "github", Deployed: true, Verified: true},
			{Alias: "box", Provider: "server"},
		}})}, []string{"rollback\n", "× box", "not rolled back", "✓ gh", "rolled back"}},
		{"restore", rotator.Summary{Verb: "restore", Keys: restoreRows("/x/dist/authama-20261111-101500.age", bundler.RestoreResult{
			Fingerprints: []bundler.FP{{Name: "work_hpc-ed25519", Fingerprint: "SHA256:abc"}}, Skipped: []string{"config"},
		})}, []string{"restore\n", "authama-20261111-101500", "SHA256:abc", "restored", "re-rendered"}},
		{"pin", rotator.Summary{Verb: "pin", Keys: pinRows([]knownhosts.HostPinResult{
			{Profile: "work", Alias: "hpc", Hostname: "hpc.example.edu", Port: 22, Status: "pinned"},
			{Profile: "work", Alias: "box", Hostname: "10.0.0.2", Port: 22, Status: "unreachable"},
			{Profile: "", Alias: "gh", Hostname: "github.com", Port: 22, Status: "already-trusted"},
		})}, []string{"pin\n", "global", "× box", "10.0.0.2:22", "not pinned", "host unreachable [1]", "already trusted", "check network"}},
	}
	for _, c := range cases {
		out := c.sum.Format()
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: the summary lacks %q:\n%s", c.name, w, out)
			}
		}
	}
}
