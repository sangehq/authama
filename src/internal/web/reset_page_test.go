package web

import (
	"regexp"
	"strings"
	"testing"
)

// The Reset page offers every way to start something again, each with what
// it resets, keeps and how to undo it, and the forms for the two it holds
// itself. A read-only server shows the cards and none of the controls.
func TestTheResetPageOffersEveryReset(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/reset").Body.String()
	for _, want := range []string{
		`href="/jobs/review?action=rotate-keys&amp;all=1"`, `href="/jobs/review?action=revoke-keys"`, `href="/snapshots#prune"`,
		`href="/security#forget"`, `href="/repair#factory-reset"`,
		`name="action" value="settings-reset"`, `name="action" value="home-init"`, `pattern="init"`,
		`name="action" value="history-reset"`, `pattern="history"`, `data-bs-target="#history-reset"`,
		"authama settings init --force", "authama init --force --backup", "authama history reset", "authama reset",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the reset page lacks %q", want)
		}
	}
	if n := strings.Count(body, "<dt>Undo</dt>"); n < 8 {
		t.Errorf("%d rows say how to undo them, want every one of the 8", n)
	}
	if !strings.Contains(body, "Cannot be undone") || !strings.Contains(body, "Undoable") {
		t.Error("the rows do not say which resets can be undone")
	}

	p, _ := fixtureHome(t)
	ro, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	roBody := signIn(t, ro).get("/reset").Body.String()
	if strings.Contains(roBody, `value="home-init"`) || strings.Contains(roBody, `href="/repair#factory-reset"`) {
		t.Error("a read-only server offers a reset control")
	}
}

// With history.permanent on, the history is never set aside: the page offers
// no history reset and says why, and the change itself refuses.
func TestAPermanentHistoryIsNotOffered(t *testing.T) {
	t.Setenv("AUTHAMA_HISTORY_PERMANENT", "true")
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/reset").Body.String()
	if strings.Contains(body, `value="history-reset"`) {
		t.Error("the page offers a history reset while history.permanent is on")
	}
	if !strings.Contains(body, "history.permanent is on") {
		t.Error("the page does not say the history is kept for good")
	}
}

// navGroupRe reads each main-menu group and its entries from the rendered page.
var navGroupRe = regexp.MustCompile(`(?s)class="nav-link dropdown-toggle" href="#navbar-([a-z]+)".*?<div class="dropdown-menu"[^>]*>(.*?)</div>\s*</li>`)

// The main menu groups pages by what they are about: SSH holds the rendered
// ssh config beside the hosts it comes from, Auth tokens is an entry of its
// own, Backups & export holds the copies, System holds Repair and Reset, and
// Actions is a menu like the others, not a button.
func TestTheMainMenuGroupsPagesByWhatTheyAreAbout(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/").Body.String()
	groups := map[string]string{}
	for _, m := range navGroupRe.FindAllStringSubmatch(body, -1) {
		groups[m[1]] = m[2]
	}
	if len(groups) < 5 {
		t.Fatalf("read %d menu groups, the pattern is not matching the menu", len(groups))
	}
	for group, hrefs := range map[string][]string{
		"actions": {"/jobs/review?action=rotate-keys", "/jobs/review?action=revoke-keys", "/reset"},
		"ssh":     {"/profiles", "/hosts", "/keys", "/vms", "/naming", "/ssh-config"},
		"auth":    {"/checklists", "/audit", "/security"},
		"backups": {"/snapshots", "/export"},
		"system":  {"/doctor", "/validate", "/repair", "/reset", "/settings", "/about"},
	} {
		for _, h := range hrefs {
			if !strings.Contains(groups[group], `href="`+h+`"`) {
				t.Errorf("menu %s lacks %s", group, h)
			}
		}
	}
	if strings.Contains(groups["auth"], `href="/tokens"`) {
		t.Error("Auth tokens is still inside Security, rather than an entry of its own")
	}
	if !regexp.MustCompile(`<li class="nav-item[^"]*">\s*<a class="nav-link" href="/tokens"`).MatchString(body) {
		t.Error("Auth tokens is not a top-level menu entry")
	}
	if strings.Contains(body, "nav-actions") {
		t.Error("the Actions menu is still styled as a button")
	}
}
