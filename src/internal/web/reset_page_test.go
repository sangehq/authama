package web

import (
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

// The main menu groups pages by what they are about (owner, 2026-11-16):
// SSH holds the estate and its tasks, Tokens has its own menu, Security the
// audit, checklists and owner, History the record and the backups, and
// System the health tools, Settings and Reset. There is no Actions menu:
// each job is under its object's Tasks, and About is in the user menu.
func TestTheMainMenuGroupsPagesByWhatTheyAreAbout(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	groups := map[string][]string{}
	for _, e := range navEntries(t, c.get("/").Body.String()) {
		if e.Href != "" { // a toggle, Running as, the tour and Sign out open no page
			groups[e.Menu] = append(groups[e.Menu], e.Href)
		}
	}
	if len(groups) < 7 {
		t.Fatalf("read %d menus, the pattern is not matching the menu", len(groups))
	}
	for group, hrefs := range map[string][]string{
		"ssh": {"/profiles", "/hosts", "/keys", "/vms", "/tools", "/services", "/git-sign", "/naming", "/ssh-config", "/termius", "/structure",
			"/hosts#add-host", "/hosts#knownhosts-init", "/keys#mint-keys",
			"/jobs/review?action=rotate-keys", "/jobs/review?action=revoke-keys", "/jobs/review?action=delete-keys"},
		"tokens":   {"/tokens", "/checklists?show=tokens", "/tokens#add-token"},
		"security": {"/audit", "/checklists", "/security"},
		"history":  {"/history", "/events", "/snapshots", "/export", "/setup"},
		"system":   {"/doctor", "/validate", "/repair", "/logs", "/settings", "/api-tokens", "/webhooks", "/reset"},
		"new":      {"/hosts#add-host", "/tokens#add-token", "/keys#mint-keys"},
		"user":     {"/about"},
		"help":     {"/integrations", "/ssh-folder"},
	} {
		if len(groups[group]) != len(hrefs) {
			t.Errorf("menu %s holds %v, want %v", group, groups[group], hrefs)
			continue
		}
		for i, h := range hrefs {
			if groups[group][i] != h {
				t.Errorf("menu %s entry %d is %s, want %s", group, i, groups[group][i], h)
			}
		}
	}
	if _, ok := groups["actions"]; ok {
		t.Error("there is still an Actions menu")
	}
}
