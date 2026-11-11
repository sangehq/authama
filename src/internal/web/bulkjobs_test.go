package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/mutation"
)

// The bulk jobs take any mix of keys, profiles and categories, and say which
// keys they will act on, and which they will not, before anything runs.
func TestBulkJobsReviewTheChosenKeys(t *testing.T) {
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	key := fixtureKey(t, c) // used by host gh, and never minted

	rev := c.change("/jobs/review", url.Values{"action": {"rotate-keys"}, "sel": {key}})
	if body := rev.Body.String(); rev.Code != http.StatusOK || !strings.Contains(body, "authama ssh rotate "+key+" --yes") {
		t.Fatalf("rotate one key: %d\n%s", rev.Code, body)
	}
	// Several selectors are carried to the run step, every one of them.
	rev = c.change("/jobs/review", url.Values{"action": {"rotate-keys"}, "sel": {"work", key}})
	if body := rev.Body.String(); rev.Code != http.StatusOK || strings.Count(body, `name="sel"`) != 2 {
		t.Fatalf("both selectors carried: %d, %d sel fields", rev.Code, strings.Count(body, `name="sel"`))
	}
	for _, tc := range []struct {
		name string
		form url.Values
		want string
	}{
		{"revoke a key never minted", url.Values{"action": {"revoke-keys"}, "sel": {key}}, "has been minted"},
		{"delete a key a host uses", url.Values{"action": {"delete-keys"}, "sel": {key}}, "used by a host"},
		{"an unknown category", url.Values{"action": {"rotate-keys"}, "sel": {"category:nosuch"}}, "no profile is filed"},
		{"a malformed selector", url.Values{"action": {"rotate-keys"}, "sel": {"../etc"}}, "not a key or profile"},
		{"nothing chosen", url.Values{"action": {"revoke-keys"}}, "choose at least one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := c.change("/jobs/review", tc.form)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%d, want 400 saying %q\n%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestTheKeysPageOffersThePickerForEachBulkAction(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/keys").Body.String()
	for _, want := range []string{`id="rotate-keys"`, `id="revoke-keys"`, `id="delete-keys"`, "data-key-picker",
		`name="sel" value="work"`, `kp-why">never minted<`, `kp-why">a host uses it<`, `data-pick-submit="rotate"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the keys page lacks %q", want)
		}
	}
}
