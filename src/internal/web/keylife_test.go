package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

// A key's lifetime is set from its page like any change: the preview shows
// the manifest and inventory diffs, confirming writes them, and a lifetime
// that is not a number of days or a preset is refused before any preview.
func TestKeyLifetimeFromThePage(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	key := fixtureKey(t, c)
	if body := c.get("/keys/" + key).Body.String(); !strings.Contains(body, `value="key-lifetime"`) {
		t.Fatal("the key page offers no lifetime form")
	}
	form := func() url.Values {
		return url.Values{"action": {"key-lifetime"}, "key": {key}, "lifetime": {"90"}}
	}
	rec := c.change("/changes/preview", form())
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "authama ssh key lifetime "+key+" 90") {
		t.Fatalf("preview: %d\n%.600s", rec.Code, body)
	}
	m := hashField.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no confirm hash:\n%.600s", body)
	}
	f := form()
	f.Set("hash", m[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.600s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(p.Manifest()); !strings.Contains(string(b), `"rotate_after_days": 90`) {
		t.Errorf("the manifest does not carry the lifetime:\n%s", b)
	}
	// The test home's keys were never minted, so there is no inventory record
	// to recount; editor's tests cover the record and its expiry date.

	for _, bad := range []string{"0", "9999", "forever", "90; rm -rf ~"} {
		f := form()
		f.Set("lifetime", bad)
		if rec := c.change("/changes/preview", f); strings.Contains(rec.Body.String(), `name="hash"`) {
			t.Errorf("lifetime %q was offered for confirmation", bad)
		}
	}
}
