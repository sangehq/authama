package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

var (
	usesLine   = regexp.MustCompile(`^\s*(?:-\s*)?uses:\s*([^\s#]+)\s*(?:#\s*(\S+))?`)
	shaRef     = regexp.MustCompile(`^[0-9a-f]{40}$`)
	majorTag   = regexp.MustCompile(`^v\d+$`)
	releaseTag = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)
)

// noReleaseUpstreams publish no releases, so a SHA with no version comment is
// the only pin there is. The estate's canonical pin checker exempts the same.
var noReleaseUpstreams = []string{"laranail/.github/"}

// actionProblems checks every workflow in dir against the estate's Actions
// rules: GitHub's and Docker's own actions float on a major tag; any other is
// pinned to a commit with its release in a comment; a push trigger filters
// tags only; and every job that runs steps has timeout-minutes. It returns the
// problems and how many workflow files it read.
func actionProblems(t *testing.T, dir string) ([]string, int) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)
		for i, line := range strings.Split(string(b), "\n") {
			m := usesLine.FindStringSubmatch(line)
			if m == nil || strings.HasPrefix(m[1], "./") {
				continue
			}
			ref, comment := m[1], m[2]
			at := strings.LastIndex(ref, "@")
			if at < 0 {
				out = append(out, fmt.Sprintf("%s:%d %s is not pinned at all", name, i+1, ref))
				continue
			}
			repo, ver := ref[:at], ref[at+1:]
			firstParty := strings.HasPrefix(repo, "actions/") || strings.HasPrefix(repo, "docker/") || strings.HasPrefix(repo, "github/")
			exempt := false
			for _, e := range noReleaseUpstreams {
				exempt = exempt || strings.HasPrefix(repo+"/", e)
			}
			switch {
			case firstParty && !majorTag.MatchString(ver):
				out = append(out, fmt.Sprintf("%s:%d %s is GitHub's or Docker's own: float it on its major tag (@vN), not %s", name, i+1, repo, ver))
			case !firstParty && !shaRef.MatchString(ver):
				out = append(out, fmt.Sprintf("%s:%d %s is a third party's: pin it to a commit SHA, not %s", name, i+1, repo, ver))
			case !firstParty && !exempt && !releaseTag.MatchString(comment):
				out = append(out, fmt.Sprintf("%s:%d %s needs its release as the comment (# x.y.z), got %q", name, i+1, repo, comment))
			}
		}
		var doc struct {
			On   map[string]any `yaml:"on"`
			Jobs map[string]struct {
				Uses    string `yaml:"uses"`
				Timeout any    `yaml:"timeout-minutes"`
			} `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(b, &doc); err != nil {
			out = append(out, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if push, ok := doc.On["push"]; ok {
			filter, _ := push.(map[string]any)
			_, branches := filter["branches"]
			_, tags := filter["tags"]
			if filter == nil || branches || !tags {
				out = append(out, fmt.Sprintf("%s: a push trigger may only filter tags (CI runs on pull_request; main gets a weekly schedule)", name))
			}
		}
		for job, j := range doc.Jobs {
			if j.Uses == "" && j.Timeout == nil {
				out = append(out, fmt.Sprintf("%s: job %s has no timeout-minutes (the default is six hours)", name, job))
			}
		}
	}
	return out, len(files)
}

// The workflows follow the Actions rules, checked here rather than only by a
// sweep elsewhere: nothing in this repository did (review R-GAP-09).
func TestWorkflowsFollowTheActionsRules(t *testing.T) {
	problems, n := actionProblems(t, filepath.Join(repoTop(t), ".github", "workflows"))
	if n < 6 {
		t.Fatalf("read %d workflow files; the glob is not finding them", n)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// The checker finds each kind of breach, so a clean result above means
// something.
func TestTheActionsCheckerCatchesEachBreach(t *testing.T) {
	dir := t.TempDir()
	bad := `on:
  push:
    branches: [main]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@0123456789abcdef0123456789abcdef01234567
      - uses: some/action@v3
      - uses: other/action@0123456789abcdef0123456789abcdef01234567
`
	if err := os.WriteFile(filepath.Join(dir, "bad.yml"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	problems, n := actionProblems(t, dir)
	got := strings.Join(problems, "\n")
	for _, want := range []string{"float it on its major tag", "pin it to a commit SHA", "needs its release as the comment", "may only filter tags", "no timeout-minutes"} {
		if !strings.Contains(got, want) {
			t.Errorf("the checker missed %q (read %d file):\n%s", want, n, got)
		}
	}
}
