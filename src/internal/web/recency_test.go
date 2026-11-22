package web

import (
	"io/fs"
	"net/http"
	"net/url"
	"os"
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

// The host page shows its key's pill, and the overview the keys labelled
// new, each with the same pill the keys pages use.
func TestKeyRecencyPillsOnTheHostAndOverviewPages(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	ref := fixtureKey(t, c)
	pill := `<span class="badge badge-info" data-recency="new">new</span>`
	if strings.Contains(c.get("/").Body.String(), `data-recency=`) {
		t.Error("the overview shows a pill before any key is new")
	}
	recordKey(t, p, ref)
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	profile, key, _ := strings.Cut(ref, "/")
	hosts, err := m.HostsForKey(manifest.KeyRef{Profile: profile, KeyName: key})
	if err != nil || len(hosts) == 0 {
		t.Fatalf("no host uses %s: %v", ref, err)
	}
	alias := hosts[0].Alias
	if host := c.get("/hosts/" + alias).Body.String(); !strings.Contains(host, pill) {
		t.Errorf("the host page for %s lacks its key's pill:\n%.3000s", alias, host)
	}
	ov := c.get("/").Body.String()
	if !strings.Contains(ov, pill) || !strings.Contains(ov, `href="/keys/`+ref+`"`) {
		t.Errorf("the overview does not list %s with its pill", ref)
	}
}

// Every recency pill is the one partial: no template draws its own, so the
// keys list, a key's page, a host's page and the overview cannot drift apart.
// Positive control: the partial itself is found. Floor: the walk must have
// read the templates.
func TestEveryRecencyPillIsThePartial(t *testing.T) {
	const partial = "templates/partials/recency_pill.html"
	files, users, own := 0, 0, false
	err := fs.WalkDir(templateFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		files++
		b, err := fs.ReadFile(templateFS, p)
		if err != nil {
			return err
		}
		s := string(b)
		if p == partial {
			own = strings.Contains(s, `data-recency="{{.Pill.Label}}"`)
			return nil
		}
		if strings.Contains(s, "data-recency=") || strings.Contains(s, "{{toneClass .Pill.Tone}}") {
			t.Errorf("%s draws a recency pill of its own; use {{template \"recency_pill\" .}}", p)
		}
		if strings.Contains(s, `{{template "recency_pill"`) {
			users++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 40 || !own {
		t.Fatalf("read %d templates (want at least 40), partial found: %v", files, own)
	}
	if users < 4 {
		t.Errorf("%d templates use the pill partial, want the keys list, key, host and overview pages", users)
	}
}

// When revoked.json cannot be read, every pill reads "unknown", and the keys
// list and the key's page say why in a notice (audit A-BUG-06).
func TestUnreadableRecordsShowAPageNotice(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	ref := fixtureKey(t, c)
	recordKey(t, p, ref)
	if strings.Contains(c.get("/keys").Body.String(), `id="records-notice"`) {
		t.Fatal("a notice with every record readable")
	}
	if err := os.WriteFile(inventory.RevokedPath(p.Inventory()), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/keys", "/keys/" + ref} {
		page := c.get(path).Body.String()
		if !strings.Contains(page, `id="records-notice"`) || !strings.Contains(page, `data-recency="unknown">unknown</span>`) {
			t.Errorf("%s: no notice or no unknown pill with revoked.json unreadable:\n%.1500s", path, page)
		}
	}
}
