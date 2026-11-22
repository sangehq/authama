package web

import (
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/events"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// strayKeyBody stands in for a private key's content: the page must never show it.
const strayKeyBody = "STRAY-PRIVATE-KEY-CONTENT-not-a-real-key\n"

// looseKey puts a key pair directly in ~/.ssh, outside the layout.
func looseKey(t *testing.T, sshDir, name string) {
	t.Helper()
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, name), []byte(strayKeyBody), 0o600); err != nil {
		t.Fatal(err)
	}
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB/22QXk5vTmOo31cReZdmvraArThvJ6FLu4g9t8PFIF stray@laptop\n"
	if err := os.WriteFile(filepath.Join(sshDir, name+".pub"), []byte(pub), 0o644); err != nil {
		t.Fatal(err)
	}
}

// /structure draws the tree and the stray key with a move select for each
// profile; a chosen move goes through the preview, and is made on apply.
func TestTheStructurePageMovesAStrayKeyThroughThePreview(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	looseKey(t, p.SSHDir, "id_stray")
	c := signIn(t, s)

	page := c.get("/structure")
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "~/.ssh/id_stray") || !strings.Contains(body, "Outside the layout") {
		t.Fatalf("/structure: %d\n%.800s", page.Code, body)
	}
	if strings.Contains(body, "STRAY-PRIVATE-KEY") {
		t.Fatal("the page shows a private key's content")
	}
	if !strings.Contains(body, `data-structure-tree`) || !strings.Contains(body, `draggable="true"`) || !strings.Contains(body, `data-drop="profile:work"`) {
		t.Fatal("the tree has no drag and drop markup")
	}
	opt := regexp.MustCompile(`<option value="(key ~/.ssh/id_stray profile work)" data-target="profile:work">`).FindStringSubmatch(html.UnescapeString(body))
	if opt == nil {
		t.Fatalf("no move into work:\n%s", body)
	}

	f := url.Values{"action": {"structure-plan"}, "move": {"", opt[1]}, "moves": {""}}
	prev := c.change("/changes/preview", f)
	pb := prev.Body.String()
	if prev.Code != http.StatusOK || !strings.Contains(pb, "profiles/work/id_stray") || !strings.Contains(pb, "config/manifest.json") {
		t.Fatalf("preview: %d\n%.1200s", prev.Code, pb)
	}
	if strings.Contains(pb, "STRAY-PRIVATE-KEY") {
		t.Fatal("the preview shows a private key's content")
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "id_stray")); err != nil {
		t.Fatal("the preview moved the key")
	}
	hash := hashField.FindStringSubmatch(pb)
	if hash == nil {
		t.Fatalf("no confirm hash:\n%s", pb)
	}
	f.Set("hash", hash[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.600s", rec.Code, rec.Body.String())
	}
	b, err := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", "id_stray"))
	if err != nil || string(b) != strayKeyBody {
		t.Fatalf("moved key: %q %v", b, err)
	}
	ops, _ := AppEngine{P: p}.History(history.Filter{Source: history.SourceWeb})
	if len(ops) != 1 || ops[0].Command != "ssh structure approve" {
		t.Errorf("recorded %+v", ops)
	}
	if body := c.get("/structure").Body.String(); strings.Contains(body, "~/.ssh/id_stray<") {
		t.Errorf("still suggested after the move")
	}
}

func TestTheStructurePageDismissesAndRefusesBadPlans(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	looseKey(t, p.SSHDir, "id_stray")
	c := signIn(t, s)
	all, err := structure.Suggest(p)
	if err != nil || len(all) == 0 {
		t.Fatalf("%v %v", all, err)
	}
	id := all[0].ID
	f := url.Values{"action": {"structure-dismiss"}, "id": {id}}
	prev := c.change("/changes/preview", f)
	hash := hashField.FindStringSubmatch(prev.Body.String())
	if hash == nil {
		t.Fatalf("dismiss preview: %d\n%.600s", prev.Code, prev.Body.String())
	}
	f.Set("hash", hash[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK {
		t.Fatalf("dismiss apply: %d", rec.Code)
	}
	cur, _, _ := structure.Current(p)
	if !cur[0].Dismissed {
		t.Fatal("not dismissed")
	}
	if body := c.get("/structure?dismissed=1").Body.String(); !strings.Contains(body, `value="structure-dismiss"`) || !strings.Contains(body, `name="undo" value="1"`) {
		t.Error("no way to show it again")
	}
	for _, bad := range []url.Values{
		{"action": {"structure-plan"}},
		{"action": {"structure-plan"}, "moves": {"key ../../etc/passwd profile work"}},
		{"action": {"structure-dismiss"}, "id": {"../x"}},
	} {
		if rec := c.change("/changes/preview", bad); rec.Code == http.StatusOK && hashField.MatchString(rec.Body.String()) {
			t.Errorf("%v was offered for confirmation", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "id_stray")); err != nil {
		t.Fatal("a dismissal moved the key")
	}
}

// The server's file scan refreshes the suggestions when ~/.ssh changes.
func TestTheEventScanRefreshesTheSuggestions(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	if s.opts.EventRoots == nil {
		s.opts.EventRoots = func(p2 paths.Paths) []events.Tracked { return events.Roots(p2, nil) }
	}
	s.scanEvents()
	looseKey(t, p.SSHDir, "id_new")
	s.scanEvents()
	st, err := structure.LoadState(p)
	if err != nil || len(st.Suggestions) != 1 || st.Suggestions[0].Path != "~/.ssh/id_new" {
		t.Fatalf("state after the scan: %+v %v", st, err)
	}
}
