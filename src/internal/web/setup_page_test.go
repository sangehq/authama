package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/setupxfer"
)

// /setup offers the secrets-free document and the commands for the
// encrypted export; the document downloads, and a pasted one is imported
// through the change preview and recorded as a web operation.
func TestTheSetupPageExportsAndImportsTheDocument(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)

	page := c.get("/setup")
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Move setup") || !strings.Contains(body, "authama setup export --encrypt --out setup.age") {
		t.Fatalf("/setup: %d\n%.600s", page.Code, body)
	}
	if !strings.Contains(body, `name="action" value="setup-import"`) {
		t.Fatal("no import form")
	}

	rec := c.get("/setup/export.json")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %v", rec.Code, rec.Header())
	}
	d, err := setupxfer.Parse(rec.Body.Bytes())
	if err != nil {
		t.Fatalf("the download is not a setup document: %v\n%s", err, rec.Body.String())
	}
	if d.Secrets || len(d.Profiles) == 0 {
		t.Fatalf("document: %+v", d)
	}

	// The same document with one more profile, as another machine would have it.
	var m manifest.Manifest
	if err := json.Unmarshal(d.Manifest, &m); err != nil {
		t.Fatal(err)
	}
	m.SetProfile("imported", manifest.Profile{KeyScope: "per_service"})
	if d.Manifest, err = json.Marshal(&m); err != nil {
		t.Fatal(err)
	}
	doc, _ := d.Marshal()
	f := url.Values{"action": {"setup-import"}, "body": {string(doc)}, "on_conflict": {"keep"}}
	prev := c.change("/changes/preview", f)
	pb := prev.Body.String()
	if prev.Code != http.StatusOK || !strings.Contains(pb, "config/manifest.json") || !strings.Contains(pb, "imported") {
		t.Fatalf("preview: %d\n%.800s", prev.Code, pb)
	}
	if b, _ := os.ReadFile(p.Manifest()); strings.Contains(string(b), `"imported"`) {
		t.Fatal("the preview wrote the manifest")
	}
	hash := hashField.FindStringSubmatch(pb)
	if hash == nil {
		t.Fatalf("no confirm hash:\n%s", pb)
	}
	f.Set("hash", hash[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.600s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(p.Manifest()); !strings.Contains(string(b), `"imported"`) {
		t.Fatal("the import was not made")
	}
	ops, _ := AppEngine{P: p}.History(history.Filter{Source: history.SourceWeb})
	if len(ops) != 1 || ops[0].Command != "setup import" {
		t.Errorf("recorded %+v", ops)
	}
}

func TestTheSetupImportRefusesWhatItCannotTrust(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for name, body := range map[string]string{
		"not a document": `{"hello": 1}`,
		"claims secrets": `{"format":"authama-setup","version":1,"secrets":true}`,
		"risky option":   `{"format":"authama-setup","version":1,"manifest":{"version":1,"defaults":{"global_options":[["ForwardAgent","yes"]]},"profiles":{}}}`,
	} {
		rec := c.change("/changes/preview", url.Values{"action": {"setup-import"}, "body": {body}, "on_conflict": {"keep"}})
		if strings.Contains(rec.Body.String(), `name="hash"`) {
			t.Errorf("%s: offered for confirmation", name)
		}
	}
	rec := c.change("/changes/preview", url.Values{"action": {"setup-import"}, "body": {`{"format":"authama-setup","version":1}`}, "on_conflict": {"sometimes"}})
	if strings.Contains(rec.Body.String(), `name="hash"`) {
		t.Error("an unknown conflict policy was offered for confirmation")
	}
}
