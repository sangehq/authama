package web

import (
	"net/url"
	"slices"
	"strings"
	"testing"
)

// Deploy, rotate and rollback reach a host through its alias by default; the
// review offers "Connect directly", which the job's command line carries as
// --direct.
func TestJobsOfferTheDirectRoute(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	key := fixtureKey(t, c)
	for _, action := range []string{"deploy", "rotate", "rollback"} {
		body := c.get("/jobs/review?action=" + action + "&key=" + url.QueryEscape(key)).Body.String()
		if !strings.Contains(body, `name="direct"`) {
			t.Errorf("the %s review has no direct switch", action)
		}
		spec, err := buildJob(url.Values{"action": {action}, "key": {key}, "direct": {"1"}}, home{p: p})
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !slices.Contains(spec.scope.Args, "--direct") {
			t.Errorf("%s --direct is not on the command line: %v", action, spec.scope.Args)
		}
		spec, _ = buildJob(url.Values{"action": {action}, "key": {key}}, home{p: p})
		if slices.Contains(spec.scope.Args, "--direct") {
			t.Errorf("%s is direct without asking: %v", action, spec.scope.Args)
		}
	}
}
