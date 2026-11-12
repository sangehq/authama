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
	// Counted in the job's own form: a fix form beside it (rotate without a
	// backup) carries the same fields.
	body := rev.Body.String()
	if i := strings.Index(body, `id="job-form"`); i >= 0 {
		body = body[i:]
		if j := strings.Index(body, "</form>"); j >= 0 {
			body = body[:j]
		}
	}
	if rev.Code != http.StatusOK || strings.Count(body, `name="sel"`) != 2 {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := c.change("/jobs/review", tc.form)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("%d, want 400 saying %q\n%s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// A bulk key job's review is where its keys are chosen: Keys, the Actions
// menu and the palette link to it, it opens with the picker and nothing
// chosen, and once keys are chosen it draws them ticked beside the review.
func TestABulkJobsReviewIsWhereItsKeysAreChosen(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	keys := c.get("/keys").Body.String()
	for _, want := range []string{`href="/jobs/review?action=rotate-keys"`, `href="/jobs/review?action=revoke-keys"`, `href="/jobs/review?action=delete-keys"`} {
		if !strings.Contains(keys, want) {
			t.Errorf("the keys page does not link %q", want)
		}
	}
	for _, gone := range []string{`id="rotate-keys"`, `id="revoke-keys"`, `id="delete-keys"`} {
		if strings.Contains(keys, gone) {
			t.Errorf("the keys page still holds the slide-in %s", gone)
		}
	}
	for action, kind := range map[string]string{"rotate-keys": "rotate", "revoke-keys": "revoke", "delete-keys": "delete"} {
		page := c.get("/jobs/review?action=" + action).Body.String()
		for _, want := range []string{"data-key-picker", `data-pick-action="` + kind + `"`, "Choose keys above", `name="sel" value="work"`,
			`data-confirm="m-job" disabled`, `method="get" action="/jobs/review" id="job-options"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: the empty review lacks %q", action, want)
			}
		}
	}
	page := c.get("/jobs/review?action=rotate-keys&sel=work&force=1").Body.String()
	for _, want := range []string{`value="work" data-pick-profile checked`, "(forced)", `name="force" value="1" checked form="job-options"`, "Before it starts"} {
		if !strings.Contains(page, want) {
			t.Errorf("the review of a chosen profile lacks %q", want)
		}
	}
	if strings.Contains(page, "Choose keys above") {
		t.Error("a review with keys chosen still says to choose them")
	}
	// A key the action cannot act on is drawn, unticked, with why.
	if del := c.get("/jobs/review?action=delete-keys").Body.String(); !strings.Contains(del, `kp-why">a host uses it<`) {
		t.Error("the delete review does not say why a key cannot be chosen")
	}
}
