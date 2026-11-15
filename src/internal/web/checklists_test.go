package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/checklist"
)

// firstKey is the fixture home's first key checklist.
func firstKey(t *testing.T, s *Server) checklist.Checklist {
	t.Helper()
	all, err := checklist.New(s.opts.Engine.Home()).All()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range all {
		if c.Kind == checklist.KindKey {
			return c
		}
	}
	t.Fatal("the fixture home has no key")
	return checklist.Checklist{}
}

func TestChecklistsListEveryKeyAndOpenOne(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	key := firstKey(t, s)
	rec := c.get("/checklists")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), key.Name) {
		t.Fatalf("/checklists: %d, %s missing", rec.Code, key.Name)
	}
	rec = c.get("/checklist?item=" + url.QueryEscape(key.ID))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Every copy outside authama is known") || !strings.Contains(body, "Mark done") {
		t.Fatalf("/checklist: %d\n%s", rec.Code, body)
	}
	if rec := c.get("/checklist?item=key:SHA256:nothing"); rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown item: %d", rec.Code)
	}
	for _, show := range []string{"open", "keys", "tokens"} {
		if rec := c.get("/checklists?show=" + show); rec.Code != http.StatusOK {
			t.Fatalf("show=%s: %d", show, rec.Code)
		}
	}
}

func TestATickIsKeptAndOnlyManualStepsTick(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	key := firstKey(t, s)
	rec := c.change("/checklist/tick", url.Values{"item": {key.ID}, "step": {"copies"}, "on": {"1"}})
	if rec.Code != http.StatusSeeOther || !strings.Contains(rec.Header().Get("Location"), "/checklist?item=") {
		t.Fatalf("tick: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	again, err := checklist.New(s.opts.Engine.Home()).Get(key.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range again.Steps {
		if st.ID == "copies" && (st.State != checklist.Done || st.Ticked == "") {
			t.Fatalf("not kept: %+v", st)
		}
	}
	// A step authama checks itself is refused, and says why.
	rec = c.change("/checklist/tick", url.Values{"item": {key.ID}, "step": {"files"}, "on": {"1"}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "cannot be ticked by hand") {
		t.Fatalf("a checked step: %d", rec.Code)
	}
	// Without the CSRF token, nothing.
	if rec := c.post("/checklist/tick", url.Values{"item": {key.ID}, "step": {"copies"}, "on": {"0"}}.Encode()); rec.Code != http.StatusForbidden {
		t.Fatalf("no CSRF: %d", rec.Code)
	}
}

func TestAReadOnlyServerShowsChecklistsWithoutTicks(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	key := firstKey(t, s)
	rec := c.get("/checklist?item=" + url.QueryEscape(key.ID))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "/checklist/tick") {
		t.Fatalf("read-only page: %d, has a tick form: %v", rec.Code, strings.Contains(rec.Body.String(), "/checklist/tick"))
	}
	if rec := c.change("/checklist/tick", url.Values{"item": {key.ID}, "step": {"copies"}, "on": {"1"}}); rec.Code == http.StatusSeeOther {
		t.Fatal("a read-only server took a tick")
	}
}

func TestTheKeyPageSummarisesItsChecklist(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	key := firstKey(t, s)
	body := c.get("/keys/" + key.Name).Body.String()
	if !strings.Contains(body, "Open the checklist") || !strings.Contains(body, "/checklist?item=") {
		t.Fatalf("the key page has no checklist card")
	}
}
