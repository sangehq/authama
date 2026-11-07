package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// postJSON posts body as the page's script does: JSON, with the CSRF token
// in its header.
func (c *client) postJSON(path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "http://"+testHost+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://"+testHost)
	r.Header.Set("X-CSRF-Token", c.s.sessions.get(c.cookie.Value).csrf)
	r.AddCookie(c.cookie)
	return do(c.s, r)
}

func (c *client) savedList(t *testing.T) savedSelections {
	t.Helper()
	var l savedSelections
	rec := c.get("/api/export/selections")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &l); err != nil {
		t.Fatal(err)
	}
	return l
}

const prdHosts = `{"version":1,"grain":"host","where":{"tag":["prd"]}}`

// The page saves to and deletes from this account's store directly, and
// lists both stores.
func TestThePageSavesItsOwnSelections(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if l := c.savedList(t); len(l.Selections) != 0 || l.SaveTo != "user" || l.ReadOnly {
		t.Fatalf("empty list = %+v", l)
	}
	if rec := c.postJSON("/api/export/selections", `{"name":"prd","selection":`+prdHosts+`}`); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(paths.ExportSelectionsDir(p), "prd.json")); err != nil {
		t.Errorf("the user file: %v", err)
	}
	l := c.savedList(t)
	if len(l.Selections) != 1 || l.Selections[0].Name != "prd" || l.Selections[0].Store != "user" || l.Selections[0].Selection.Where.Tag[0] != "prd" {
		t.Errorf("list = %+v", l)
	}
	for body, want := range map[string]string{
		`{"name":"Prd","selection":` + prdHosts + `}`:           "selection name",
		`{"name":"hosts","selection":` + prdHosts + `}`:         "built-in list",
		`{"name":"x","selection":{"version":2,"grain":"host"}}`: "version 2",
		`not json`: "unreadable",
	} {
		if rec := c.postJSON("/api/export/selections", body); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s, want 400 with %q", body, rec.Code, rec.Body.String(), want)
		}
	}
	if rec := c.postJSON("/api/export/selections/delete", `{"name":"prd"}`); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := c.postJSON("/api/export/selections/delete", `{"name":"prd"}`); rec.Code != http.StatusNotFound {
		t.Errorf("delete again: %d", rec.Code)
	}
	// Without the CSRF token, nothing is saved.
	r := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/api/export/selections", strings.NewReader(`{"name":"x","selection":`+prdHosts+`}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://"+testHost)
	r.AddCookie(c.cookie)
	if rec := do(s, r); rec.Code == http.StatusOK {
		t.Error("a save without the CSRF token was accepted")
	}
}

// The manifest's store changes through the preview and its confirmation.
func TestTheManifestsSelectionsAreAChange(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	before, _ := os.ReadFile(p.Manifest())
	f := url.Values{"action": {"export-selection-save"}, "name": {"shared"}, "selection": {prdHosts}}
	rec := c.change("/changes/preview", f)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "config/manifest.json") || !strings.Contains(body, "export selections save shared") {
		t.Fatalf("preview: %d\n%s", rec.Code, body)
	}
	if after, _ := os.ReadFile(p.Manifest()); string(after) != string(before) {
		t.Fatal("the preview wrote the manifest")
	}
	m := hashField.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no confirm hash:\n%s", body)
	}
	f.Set("hash", m[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%s", rec.Code, rec.Body.String())
	}
	if l := c.savedList(t); len(l.Selections) != 1 || l.Selections[0].Store != "manifest" {
		t.Fatalf("after apply: %+v", l)
	}
	// And out again.
	f = url.Values{"action": {"export-selection-delete"}, "name": {"shared"}}
	m = hashField.FindStringSubmatch(c.change("/changes/preview", f).Body.String())
	if m == nil {
		t.Fatal("no confirm hash for the delete")
	}
	f.Set("hash", m[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK {
		t.Fatalf("delete apply: %d", rec.Code)
	}
	// The fixture is hand-written, so any save reformats it: what matters is
	// that no export section is left behind.
	if after, _ := os.ReadFile(p.Manifest()); strings.Contains(string(after), `"export"`) {
		t.Errorf("saving then deleting left an export section:\n%s", after)
	}
	if rec := c.change("/changes/preview", url.Values{"action": {"export-selection-save"}, "name": {"x"}, "selection": {"{}"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("a bad selection in the form: %d", rec.Code)
	}
}

// Read-only: the list is served and says so; nothing can be saved.
func TestReadOnlySavesNoSelection(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	if l := c.savedList(t); !l.ReadOnly {
		t.Error("read-only not said")
	}
	for _, path := range []string{"/api/export/selections", "/api/export/selections/delete"} {
		if rec := c.postJSON(path, `{"name":"prd","selection":`+prdHosts+`}`); rec.Code == http.StatusOK {
			t.Errorf("read-only %s: %d", path, rec.Code)
		}
	}
	if strings.Contains(c.get("/export").Body.String(), "data-export-save-name") {
		t.Error("read-only /export offers saving")
	}
}

// Two selections compared over HTTP: matched on the columns they share.
func TestTwoSelectionsCompare(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	a := selectionParam(t, `{"version":1,"grain":"host","columns":[{"field":"alias"},{"field":"user"}]}`)
	b := selectionParam(t, `{"version":1,"grain":"host","where":{"exclude":"gh"},"columns":[{"field":"alias"}]}`)
	rec := c.get("/exports/compare?a=" + a + "&b=" + b)
	var got struct {
		On         []string   `json:"on"`
		OnlyA      [][]string `json:"only_a"`
		OnlyACount int        `json:"only_a_count"`
		Both       int        `json:"both"`
		KeptB      int        `json:"kept_b"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if strings.Join(got.On, ",") != "alias" || got.Both != 0 || got.OnlyACount != 1 || got.OnlyA[0][0] != "gh" || got.KeptB != 0 {
		t.Errorf("compare = %+v", got)
	}
	for q, want := range map[string]string{
		"?a=" + a:            "b: give ?preset= or ?s=",
		"?a=" + a + "&b=***": "b: the selection is not base64url",
		"?a=" + a + "&b=" + selectionParam(t, `{"version":1,"grain":"host","columns":[{"field":"port"}]}`): "no column in common",
	} {
		if rec := c.get("/exports/compare" + q); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s, want %q", q, rec.Code, rec.Body.String(), want)
		}
	}
}
