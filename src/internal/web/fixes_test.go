package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fixLiteral is a fix command written into an overview row, in either of the
// two shapes overview.go uses: `Fix: "…"` in a literal, or `row.Fix = "…"`.
var fixLiteral = regexp.MustCompile(`\bFix(?::|\s*=)\s*"([^"]+)"`)

// Every fix the overview can show is either done from the web UI or listed as
// CLI-only with a reason, and neither table names a fix the overview no longer
// shows. Read from overview.go's source, so a row the test home never
// produces (dev mode, a drifted config) is held to it too.
func TestEveryOverviewFixIsPerformedOrCLIOnly(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "services", "overview", "overview.go"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range fixLiteral.FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	if len(seen) < 8 {
		t.Fatalf("found %d fix commands in overview.go; the scan is not reading it", len(seen))
	}
	for fix := range seen {
		_, web := overviewFixes[fix]
		_, cli := cliOnlyFixes[fix]
		switch {
		case web && cli:
			t.Errorf("fix %q is in both overviewFixes and cliOnlyFixes", fix)
		case !web && !cli:
			t.Errorf("fix %q has no web counterpart: map it in overviewFixes or say why in cliOnlyFixes", fix)
		}
	}
	for fix, a := range overviewFixes {
		if !seen[fix] {
			t.Errorf("overviewFixes maps %q, which overview.go no longer shows", fix)
		}
		if (a.Action == "") == (a.Page == "") {
			t.Errorf("fix %q needs exactly one of an action or a page", fix)
		}
		if _, ok := actionFields[a.Action]; a.Action != "" && !ok {
			t.Errorf("fix %q previews %q, which is not a change kind", fix, a.Action)
		}
		if a.Label == "" {
			t.Errorf("fix %q has no button label", fix)
		}
	}
	for fix := range cliOnlyFixes {
		if !seen[fix] {
			t.Errorf("cliOnlyFixes lists %q, which overview.go no longer shows", fix)
		}
	}
}

// A page a fix opens is one the server serves.
func TestEveryOverviewFixPageOpens(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	pages := 0
	for fix, a := range overviewFixes {
		if a.Page == "" {
			continue
		}
		pages++
		path, _, _ := strings.Cut(a.Page, "#")
		if rec := c.get(path); rec.Code != 200 {
			t.Errorf("fix %q opens %s, which answers %d", fix, path, rec.Code)
		}
	}
	if pages == 0 {
		t.Fatal("no fix opens a page; the loop checked nothing")
	}
}

// stateCell is a Fix cell of the overview's state table.
var stateCell = regexp.MustCompile(`(?s)<td class="fix-cell"><code>([^<]+)</code>(.*?)</td>`)

// On the overview, a row that needs attention shows its command and, where the
// web UI can do the fix, a button for it: a preview form for a change, a link
// for a page. A read-only server shows the command alone.
func TestOverviewFixesArePressable(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/").Body.String()
	cells := stateCell.FindAllStringSubmatch(body, -1)
	if len(cells) == 0 {
		t.Fatalf("the test home's overview shows no fix, so this test checks nothing:\n%s", body)
	}
	for _, m := range cells {
		fix, rest := m[1], m[2]
		a, ok := overviewFixes[fix]
		switch {
		case !ok:
			if strings.Contains(rest, "<button") || strings.Contains(rest, "<a ") {
				t.Errorf("fix %q is CLI-only and has a control: %s", fix, rest)
			}
		case a.Action != "":
			if !strings.Contains(rest, `name="action" value="`+a.Action+`"`) || !strings.Contains(rest, "/changes/preview") {
				t.Errorf("fix %q has no preview form for %q: %s", fix, a.Action, rest)
			}
		default:
			if !strings.Contains(rest, `href="`+a.Page+`"`) {
				t.Errorf("fix %q does not link to %s: %s", fix, a.Page, rest)
			}
		}
	}
}
