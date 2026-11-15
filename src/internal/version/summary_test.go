package version

import "testing"

// `authama version` names the commit and build date when Go recorded them,
// as the installation docs say; it printed only the version, so a build from
// a branch reported "dev" and nothing else (review R-GAP-08, R-DOC-05).
func TestSummaryNamesTheCommitAndDate(t *testing.T) {
	v, c, d := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = v, c, d })
	Version, Commit, Date = "v0.1.7", "abc1234", "2026-11-14T05:00:00Z"
	if got := Summary(); got != "v0.1.7 (abc1234, 2026-11-14)" {
		t.Errorf("Summary() = %q", got)
	}
	Commit, Date = "", ""
	if got := Summary(); got != "v0.1.7" {
		t.Errorf("Summary() with no commit = %q", got)
	}
	Commit, Date = "abc1234", ""
	if got := Summary(); got != "v0.1.7 (abc1234)" {
		t.Errorf("Summary() with no date = %q", got)
	}
}
