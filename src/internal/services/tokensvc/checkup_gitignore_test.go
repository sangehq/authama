package tokensvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/tokens"
)

// gitignoreMissing runs the checkup against a .gitignore holding rules and
// returns the secret file names it reports as not covered.
func gitignoreMissing(t *testing.T, rules string) []string {
	t.Helper()
	e, dir := sandbox(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(rules), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := e.Checkup(tokens.NewSet(), "")
	for _, f := range out {
		if strings.HasSuffix(f.Path, ".gitignore") {
			return f.Missing
		}
	}
	return nil
}

// A rule covers a secret file only when it ignores it: a negation un-ignores
// it, and a longer name is another file (audit B-02).
func TestGitignoreCoverageFollowsGitignoreSemantics(t *testing.T) {
	all := "auth.json, .npmrc, .git-credentials, .netrc"
	cases := []struct {
		name, rules, want string
	}{
		{"negations un-ignore", "!.npmrc\n!auth.json\n!.git-credentials\n!.netrc\n", all},
		{"longer names are other files", ".npmrc.example\nauth.json.dist\n.git-credentials.bak\n.netrc~\n", all},
		{"the last matching rule wins", "*\n!.npmrc\n", ".npmrc"},
		{"re-ignored after a negation", "!auth.json\nauth.json\n.npmrc\n.git-credentials\n.netrc\n", ""},
		{"a directory rule does not cover a file", "auth.json/\n.npmrc\n.git-credentials\n.netrc\n", "auth.json"},
		{"a rule into a subdirectory does not cover the root file", "sub/auth.json\n.npmrc\n.git-credentials\n.netrc\n", "auth.json"},
		{"anchored, globbed and ** rules cover", "/auth.json\n.npm*\n**/.git-credentials\n.net?c\n", ""},
		{"an escaped ! is a literal name", "\\!auth.json\n.npmrc\n.git-credentials\n.netrc\n", "auth.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := strings.Join(gitignoreMissing(t, c.rules), ", "); got != c.want {
				t.Errorf("rules %q: missing = %q, want %q", c.rules, got, c.want)
			}
		})
	}
}
