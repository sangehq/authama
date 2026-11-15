package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// recordKey gives the fixture key an inventory record minted now, as a
// keygen would, so it has a recency to show.
func recordKey(t *testing.T, p paths.Paths, ref string) string {
	t.Helper()
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	profile, key, _ := strings.Cut(ref, "/")
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		t.Fatal(err)
	}
	today := inventory.Today()
	fp := "SHA256:recency-fixture"
	inv.Record(fp, inventory.KeyRecord{Profile: profile, Path: m.IdentityFile(profile, key), Type: "ed25519",
		Created: &today, RotateAfterDays: 365})
	inv.NoteAdded(fp, inventory.OriginGenerated, time.Now().Add(-time.Hour))
	if err := inv.Save(p.Inventory()); err != nil {
		t.Fatal(err)
	}
	return fp
}

// A fresh key carries a "new" pill, as text, on the keys list and its page;
// the page lists each event with a machine-readable <time>; marking it seen
// goes through a previewed change and takes the label away; a revoked key's
// pill is muted.
func TestKeyRecencyPills(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	ref := fixtureKey(t, c)
	fp := recordKey(t, p, ref)

	list := c.get("/keys").Body.String()
	if !strings.Contains(list, `<span class="badge badge-info" data-recency="new">new</span>`) {
		t.Fatalf("the keys list has no new pill:\n%.2000s", list)
	}
	page := c.get("/keys/" + ref).Body.String()
	for _, want := range []string{`data-recency="new">new</span>`, `<time datetime="`, "1 hour ago", `value="key-ack"`, `badge badge-info">fresh</span>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the key page lacks %q", want)
		}
	}

	form := url.Values{"action": {"key-ack"}, "key": {ref}}
	rec := c.change("/changes/preview", form)
	m := hashField.FindStringSubmatch(rec.Body.String())
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("preview: %d\n%.600s", rec.Code, rec.Body.String())
	}
	form.Set("hash", m[1])
	if rec := c.change("/changes/apply", form); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.600s", rec.Code, rec.Body.String())
	}
	page = c.get("/keys/" + ref).Body.String()
	if strings.Contains(page, `data-recency="new"`) || strings.Contains(page, `value="key-ack"`) {
		t.Error("the key is still new after it was marked seen")
	}
	if rec := c.change("/changes/preview", url.Values{"action": {"key-ack"}, "key": {"../etc"}}); strings.Contains(rec.Body.String(), `name="hash"`) {
		t.Error("a key that is not profile/key was offered for confirmation")
	}

	revs := inventory.Revocations{fp: {Key: ref, RevokedOn: inventory.Today()}}
	if err := revs.Save(inventory.RevokedPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
	if page := c.get("/keys/" + ref).Body.String(); !strings.Contains(page, `<span class="badge badge-muted" data-recency="revoked">revoked</span>`) {
		t.Error("a revoked key's pill is not muted")
	}
}
