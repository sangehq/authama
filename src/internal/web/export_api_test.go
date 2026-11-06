package web

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Every built-in preset, as CSV, is byte for byte what the list's own CSV
// URL answers: the guard that lets the old URLs move onto the pipeline.
func TestEveryPresetIsItsListsCSVOverHTTP(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	names := []string{"hosts", "keys", "profiles", "tokens", "history", "events", "snapshots", "deployments"}
	if len(names) != len(csvLists) {
		t.Fatalf("%d presets compared, %d lists served", len(names), len(csvLists))
	}
	for _, n := range names {
		old := c.get("/exports/" + n + ".csv")
		run := c.get("/exports/run?preset=" + n + "&format=csv")
		if old.Code != http.StatusOK || run.Code != http.StatusOK {
			t.Fatalf("%s: old %d, run %d: %s", n, old.Code, run.Code, run.Body.String())
		}
		if run.Body.String() != old.Body.String() {
			t.Errorf("%s differs:\n--- /exports/%s.csv\n%s--- preset\n%s", n, n, old.Body.String(), run.Body.String())
		}
		if got := run.Header().Get("Content-Disposition"); got != `attachment; filename="authama-`+n+`.csv"` {
			t.Errorf("%s: Content-Disposition %q", n, got)
		}
	}
}

func selectionParam(t *testing.T, sel string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(sel))
}

func TestASelectionDownloadsInAnyFormat(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	sel := selectionParam(t, `{"version":1,"grain":"host","columns":[{"field":"alias"},{"field":"port"},{"field":"key.type","as":"key_type"}]}`)
	rec := c.get("/exports/run?s=" + sel + "&format=json")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("json: %d %s", rec.Code, rec.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["alias"] != "gh" ||
		rows[0]["port"] != float64(22) || rows[0]["key_type"] != "ed25519" {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="authama-hosts.json"` {
		t.Errorf("Content-Disposition %q", got)
	}
	for _, f := range []string{"csv", "tsv", "jsonl", "yaml", "php", "js", "markdown", "html"} {
		if rec := c.get("/exports/run?s=" + sel + "&format=" + f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "gh") {
			t.Errorf("%s: %d %s", f, rec.Code, rec.Body.String())
		}
	}
}

func TestThePreviewOffersFieldsFacetsAndFormats(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	var p exportPreview
	if err := json.Unmarshal(c.get("/exports/run?preset=hosts&preview=1").Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Grain != "host" || p.Total != 1 || p.Kept != 1 || len(p.Rows) != 1 || len(p.Stages) != 5 {
		t.Errorf("preview = %+v", p)
	}
	if len(p.Formats) != 9 || len(p.Fields) < 20 {
		t.Errorf("%d formats, %d fields", len(p.Formats), len(p.Fields))
	}
	if prof := p.Facets["profile"]; len(prof) != 1 || prof[0].Value != "work" || prof[0].Rows != 1 {
		t.Errorf("profile facet = %+v", p.Facets["profile"])
	}
	if keys := p.Facets["key"]; len(keys) != 1 || keys[0].Profile != "work" {
		t.Errorf("key facet = %+v", p.Facets["key"])
	}
}

func TestABadRequestSaysWhy(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for url, want := range map[string]string{
		"/exports/run":                          "give ?preset= or ?s=",
		"/exports/run?preset=planets":           "no preset named planets",
		"/exports/run?preset=hosts&s=e30":       "not both",
		"/exports/run?s=***":                    "not base64url",
		"/exports/run?preset=hosts&format=xlsx": "unknown format",
		"/exports/run?s=" + base64.RawURLEncoding.EncodeToString([]byte(`{"version":2,"grain":"host"}`)):                              "version 2",
		"/exports/run?s=" + base64.RawURLEncoding.EncodeToString([]byte(`{"version":1,"grain":"host","columns":[{"field":"nope"}]}`)): "no field",
	} {
		rec := c.get(url)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: %d %s; want 400 with %q", url, rec.Code, rec.Body.String(), want)
		}
	}
	if rec := c.get("/exports/run?s=" + strings.Repeat("A", MaxSelection+1)); rec.Code != http.StatusBadRequest {
		t.Errorf("an oversized selection: %d", rec.Code)
	}
}

// The dataset over HTTP holds the home's records and never a private key.
func TestTheDatasetIsServedWithoutSecrets(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	rec := c.get("/api/dataset")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, privateMarker) || strings.Contains(body, "secret\\n") {
		t.Fatal("private key material reached /api/dataset")
	}
	var ds struct {
		Hosts []struct{ Alias string }   `json:"hosts"`
		Ops   []struct{ Command string } `json:"ops"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ds); err != nil || len(ds.Hosts) != 1 || len(ds.Ops) != 1 {
		t.Errorf("dataset = %+v, %v", ds, err)
	}
	var core struct {
		Ops []any `json:"ops"`
	}
	_ = json.Unmarshal(c.get("/api/dataset?parts=core").Body.Bytes(), &core)
	if len(core.Ops) != 0 {
		t.Errorf("parts=core read %d operations", len(core.Ops))
	}
}

// Signed out, nothing is served.
func TestTheExportAPINeedsASession(t *testing.T) {
	s, _, _ := newTestServer(t)
	for _, u := range []string{"/api/dataset", "/exports/run?preset=hosts"} {
		rec := do(s, httptestGet(u))
		if rec.Code == http.StatusOK {
			t.Errorf("%s answered 200 without a session", u)
		}
	}
}
