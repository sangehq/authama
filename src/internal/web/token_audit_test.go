package web

import (
	"testing"

	"github.com/sangehq/authama/src/internal/services/tokensvc"
)

// The token audit is laid out as what to fix, what passed (with its note),
// and the tools with no file. A file others can read is one problem, though
// the audit says so in two lines.
func TestTheTokenAuditIsLaidOutForTheCard(t *testing.T) {
	a := layOutAudit([]tokensvc.Finding{
		{Level: "ok", Target: "composer", Path: "/h/.composer/auth.json", Mode: "0600"},
		{Level: "note", Target: "composer", Text: "a present auth.json OVERRIDES $COMPOSER_AUTH."},
		{Level: "warn", Target: "npm", Path: "/h/.npmrc", Mode: "0644"},
		{Level: "warn", Target: "npm", Path: "/h/.npmrc", Mode: "0644", Text: "world/group-readable"},
		{Level: "absent", Target: "pypi", Path: "/h/.pypirc"},
		{Level: "warn", Path: "/p/.gitignore", Missing: []string{".npmrc", ".netrc"}},
	})
	if len(a.Problems) != 2 {
		t.Errorf("problems = %+v, want the npm file once and the .gitignore", a.Problems)
	}
	if len(a.Passed) != 1 || a.Passed[0].Note == "" || a.Passed[0].Mode != "0600" {
		t.Errorf("passed = %+v, want composer with its mode and its note", a.Passed)
	}
	if len(a.Absent) != 1 || a.Absent[0].Target != "pypi" {
		t.Errorf("absent = %+v", a.Absent)
	}
}
