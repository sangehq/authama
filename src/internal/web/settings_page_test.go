package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/settings"
)

// The Settings page groups settings by section, each with an edit form whose
// input fits its kind, and saving one goes through a preview into
// authama.json.
func TestSettingsCanBeEditedFromTheirPage(t *testing.T) {
	t.Setenv("AUTHAMA_GIT_MODE", "")
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	body := c.get("/settings").Body.String()
	for _, want := range []string{`id="settings-backup"`, `id="settings-web"`, `id="setting-git.mode"`,
		`name="action" value="settings-set"`, `type="number" id="v-AUTHAMA_WEB_PORT"`,
		`<select class="form-select setting-input" id="v-AUTHAMA_GIT_MODE"`, `<option value="true"`, "data-settings-filter",
		`name="action" value="backup-key"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the settings page lacks %q", want)
		}
	}
	f := url.Values{"action": {"settings-set"}, "key": {"git.mode"}, "value": {"ssh"}}
	prev := c.change("/changes/preview", f).Body.String()
	m := hashField.FindStringSubmatch(prev)
	if m == nil {
		t.Fatalf("no preview hash:\n%s", shorten(prev, 400))
	}
	f.Set("hash", m[1])
	c.change("/changes/apply", f)
	if v, ok := settings.FileValue(p.SettingsFile(), "git.mode"); !ok || v != "ssh" {
		t.Errorf("git.mode in authama.json = %q, %v; want ssh", v, ok)
	}
	if after := c.get("/settings").Body.String(); !strings.Contains(after, `name="action" value="settings-unset"`) {
		t.Error("a saved setting offers no way back to its default")
	}
}

// A job's check that fails offers its fix: a rotate with no backup recipient
// offers to create a backup key, or to set one in Settings.
func TestAFailedJobCheckOffersItsFix(t *testing.T) {
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "")
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	body := c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {fixtureKey(t, c)}}).Body.String()
	for _, want := range []string{`name="action" value="backup-key"`, "Create a backup key", `href="/settings#setting-backup.age_recipient"`, "Use a key you have"} {
		if !strings.Contains(body, want) {
			t.Errorf("the failed check lacks %q", want)
		}
	}
}
