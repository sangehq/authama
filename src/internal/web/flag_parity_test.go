package web

import (
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/rotator"
)

// firstHost is a profile and one of its hosts from the test manifest.
func firstHost(t *testing.T, path string) (string, string) {
	t.Helper()
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range m.ProfileNames() {
		if hs := m.Profiles[name].Hosts; len(hs) > 0 {
			return name, hs[0].Alias
		}
	}
	t.Fatal("the test manifest has no host")
	return "", ""
}

// The smaller CLI flags the forms now carry land on the change's command
// line, so what the page does is what the CLI would.
func TestTheFormsCarryTheCLIFlags(t *testing.T) {
	_, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	profile, _ := firstHost(t, p.Manifest())
	h := home{p: p}
	cases := []struct {
		name string
		form url.Values
		want []string
	}{
		{"profile add --code", url.Values{"action": {"profile-add"}, "name": {"fresh"}, "code": {"fr"}}, []string{"--code=fr"}},
		{"key add --rotate-after-days", url.Values{"action": {"key-add"}, "profile": {profile}, "key": {"spare2"}, "rotate_after_days": {"90"}}, []string{"--rotate-after-days=90"}},
		{"key ack --all", url.Values{"action": {"key-ack"}, "all": {"1"}}, []string{"--all"}},
		{"key ack --category", url.Values{"action": {"key-ack"}, "all": {"1"}, "category": {"clients"}}, []string{"--category=clients"}},
		{"host apply --tag --reconcile", url.Values{"action": {"host-apply"}, "profile": {profile}, "specs": {"web9 203.0.113.9 deploy"}, "tags": {"prod"}, "reconcile": {"1"}}, []string{"--reconcile", "--no-pin"}},
		{"vm sync --no-probe", url.Values{"action": {"vm-sync"}, "no_probe": {"1"}}, []string{"--no-probe"}},
		{"repair --area", url.Values{"action": {"repair"}, "area_ssh": {"1"}}, []string{"--area=ssh"}},
	}
	for _, c := range cases {
		ch, err := buildChange(c.form, h)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		args := strings.Join(ch.Args, " ")
		if !strings.Contains(args, strings.Join(c.want, " ")) {
			t.Errorf("%s: args %q, want %q in them", c.name, args, strings.Join(c.want, " "))
		}
	}
	// host add --env --role with no alias generates one, as the CLI does,
	// and says what is missing when the profile has no code to name it by.
	_, err := buildChange(url.Values{"action": {"host-add"}, "profile": {profile}, "env": {"prod"}, "role": {"web"}, "hostname": {"203.0.113.7"}, "user": {"deploy"}}, h)
	if err == nil || !strings.Contains(err.Error(), "--code") {
		t.Errorf("a generated alias without a code: %v", err)
	}
	// A rotation period out of range is refused, as the CLI refuses it.
	if _, err := buildChange(url.Values{"action": {"key-add"}, "profile": {profile}, "key": {"x"}, "rotate_after_days": {"-1"}}, h); err == nil {
		t.Error("a negative rotation period was taken")
	}
}

// host delete and profile delete with --revoke or --purge are jobs; without
// --revoke they reach no server, so the sandbox runs them.
func TestDeletingWithOptionsIsAJob(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	profile, alias := firstHost(t, p.Manifest())
	spec, err := buildJob(url.Values{"action": {"delete-host"}, "profile": {profile}, "alias": {alias}, "purge": {"1"}}, home{p: p})
	if err != nil {
		t.Fatal(err)
	}
	if spec.scope.Command != "host delete" || !slices.Contains(spec.scope.Args, "--purge") || !spec.backup || !spec.destroy {
		t.Errorf("delete-host: %+v", spec.scope)
	}
	spec, err = buildJob(url.Values{"action": {"delete-profile"}, "name": {profile}}, home{p: p})
	if err != nil {
		t.Fatal(err)
	}
	if spec.scope.Command != "profile delete" || slices.Contains(spec.scope.Args, "--purge") {
		t.Errorf("delete-profile: %+v", spec.scope)
	}
	if _, err := buildJob(url.Values{"action": {"delete-host"}, "profile": {profile}, "alias": {"nope"}}, home{p: p}); err == nil {
		t.Error("an unknown host was taken")
	}
	c := signIn(t, s)
	body := c.get("/jobs/review?action=delete-host&profile=" + profile + "&alias=" + alias).Body.String()
	if !strings.Contains(body, `name="revoke"`) || !strings.Contains(body, `name="purge"`) {
		t.Errorf("the review offers no revoke or purge:\n%.2000s", body)
	}
}

// rotate --failed: the review chooses the keys the last run left.
func TestTheRotateReviewChoosesTheFailedKeys(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	if b := c.get("/jobs/review?action=rotate-keys&failed=1"); !strings.Contains(b.Body.String(), "Nothing to run again") {
		t.Errorf("an empty failed list: %d", b.Code)
	}
	key := fixtureKey(t, c)
	if err := os.MkdirAll(p.StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := rotator.SaveFailed(p, []string{key}); err != nil {
		t.Fatal(err)
	}
	body := c.get("/jobs/review?action=rotate-keys&failed=1").Body.String()
	if !strings.Contains(body, `name="sel" value="`+key+`"`) {
		t.Errorf("the failed key is not chosen:\n%.3000s", body)
	}
}
