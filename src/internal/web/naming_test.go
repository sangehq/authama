package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The Naming page is `ssh naming show` and `ssh naming check`, and offers
// `naming set`, `naming key-template` and a profile's code.
func TestNamingPageShowsTheRulesAndOffersTheirChanges(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := signIn(t, s).get("/naming").Body.String()
	for _, want := range []string{
		"{code}-{env}-{role}-{nn}", "{profile}_{alias}-{algo}", // the default templates
		`id="naming-rules-table"`, `id="naming-profiles-table"`, "<code>work</code>",
		"authama ssh naming show", "authama ssh naming check", "server(s) checked",
		`value="naming-set"`, `value="naming-key-template"`, `value="profile-code"`,
		`id="naming-rules"`, `id="naming-key"`, `id="naming-p0"`, `id="code-p0"`,
		`<input type="hidden" name="profile" value="work">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Naming page has no %q", want)
		}
	}
}

// The rules form posts every field; only the ones that differ from what the
// scope stores become flags, so the preview reads as the command a person
// would type.
func TestNamingChangesPreviewAsTheirCommands(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	unchanged := url.Values{"alias": {""}, "envs": {""}, "roles": {""}, "enforce": {""}, "start": {"0"}, "code_fallback": {""}}
	with := func(action string, extra url.Values, base url.Values) url.Values {
		f := url.Values{"action": {action}}
		for k, v := range base {
			f[k] = v
		}
		for k, v := range extra {
			f[k] = v
		}
		return f
	}
	for _, tc := range []struct {
		form url.Values
		cmd  string
	}{
		{with("naming-set", url.Values{"envs": {"prd, stg"}, "enforce": {"warn"}}, unchanged),
			"authama ssh naming set --envs=prd,stg --enforce=warn"},
		{with("naming-set", url.Values{"profile": {"work"}, "alias": {"{code}-{role}-{nn}"}, "start": {"1"}}, unchanged),
			"authama ssh naming set --profile work --alias={code}-{role}-{nn} --start=1"},
		{with("naming-key-template", url.Values{"key_name": {"{profile}_{alias}_k-{algo}"}}, nil),
			"authama ssh naming key-template {profile}_{alias}_k-{algo} --yes"},
		{with("naming-key-template", url.Values{"key_name": {"{profile}_{alias}_k-{algo}"}, "profile": {"work"}}, nil),
			"authama ssh naming key-template {profile}_{alias}_k-{algo} --yes --profile work"},
		{with("profile-code", url.Values{"name": {"work"}, "code": {"wk"}}, nil), "authama ssh profile edit work --code=wk"},
	} {
		rec := c.change("/changes/preview", tc.form)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.cmd) {
			t.Errorf("%v: %d, want %q\n%s", tc.form, rec.Code, tc.cmd, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "--roles") || strings.Contains(rec.Body.String(), "--code-fallback") {
			t.Errorf("%v: a field left as it is became a flag", tc.form)
		}
	}
}

// Every value is checked before a preview: names, never paths; only the
// values the command takes; and a form that changes nothing is refused.
func TestNamingChangesRefuseWhatTheCommandWould(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, f := range []url.Values{
		{"action": {"naming-set"}, "start": {"0"}}, // nothing changes
		{"action": {"naming-set"}, "enforce": {"loud"}},
		{"action": {"naming-set"}, "start": {"2"}},
		{"action": {"naming-set"}, "code_fallback": {"alias"}},
		{"action": {"naming-set"}, "envs": {"prd,../x"}},
		{"action": {"naming-set"}, "envs": {"-oProxyCommand=x"}},
		{"action": {"naming-set"}, "profile": {"nobody"}, "enforce": {"warn"}},
		{"action": {"naming-set"}, "profile": {"../work"}, "enforce": {"warn"}},
		{"action": {"naming-key-template"}, "key_name": {""}},
		{"action": {"naming-key-template"}, "key_name": {"{alias}"}},
		{"action": {"profile-code"}, "name": {"work"}, "code": {"WK"}},
		{"action": {"profile-code"}, "name": {"work"}, "code": {""}}, // already empty
		{"action": {"profile-code"}, "name": {"nobody"}, "code": {"x"}},
	} {
		if rec := c.change("/changes/preview", f); rec.Code != http.StatusBadRequest {
			t.Errorf("%v was accepted: %d", f, rec.Code)
		}
	}
}

// A read-only server shows the rules and offers no change.
func TestReadOnlyNamingPageOffersNoChange(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	body := signIn(t, s).get("/naming").Body.String()
	if !strings.Contains(body, `id="naming-rules-table"`) {
		t.Error("a read-only Naming page shows no rules")
	}
	for _, action := range []string{"naming-set", "naming-key-template", "profile-code"} {
		if strings.Contains(body, action) {
			t.Errorf("a read-only Naming page offers %s", action)
		}
	}
}
