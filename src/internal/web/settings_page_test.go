package web

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/settings"
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

// A fix started from a job's review carries the job through its preview, so
// the done page offers to review that job again rather than leaving you on a
// review that still shows the check failing.
func TestAFixTakesYouBackToTheJob(t *testing.T) {
	t.Setenv("AUTHAMA_GIT_MODE", "")
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	f := url.Values{"action": {"settings-set"}, "key": {"git.mode"}, "value": {"ssh"},
		"then_action": {"rotate-keys"}, "then_sel": {"work"}, "then_no_backup": {"1"}, "then_bogus": {"x"}}
	prev := c.change("/changes/preview", f).Body.String()
	if !strings.Contains(prev, `name="then_action" value="rotate-keys"`) || strings.Contains(prev, "then_bogus") {
		t.Fatalf("the preview does not carry the job (and only the job):\n%s", shorten(prev, 600))
	}
	m := hashField.FindStringSubmatch(prev)
	if m == nil {
		t.Fatal("no preview hash")
	}
	f.Set("hash", m[1])
	done := c.change("/changes/apply", f).Body.String()
	for _, want := range []string{"Review the job again", `action="/jobs/review"`, `name="action" value="rotate-keys"`, `name="sel" value="work"`, `name="no_backup" value="1"`} {
		if !strings.Contains(done, want) {
			t.Errorf("the done page lacks %q", want)
		}
	}
}

// A rotation's review offers a dry run, and a rotation without the backup:
// the dry run changes nothing and needs no backup; skipping the backup drops
// the backup checks and says what it means.
func TestARotationCanBeTestedOrRunWithoutABackup(t *testing.T) {
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "")
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	key := fixtureKey(t, c)
	body := c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}}).Body.String()
	for _, want := range []string{`name="check" value="1"`, "Dry run: test every key", "Skip the encrypted backup", `name="no_backup" value="1"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the review lacks %q", want)
		}
	}
	dry := c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}, "check": {"1"}}).Body.String()
	for _, want := range []string{"Dry run: rotate " + key, "changes nothing", "--check"} {
		if !strings.Contains(dry, want) {
			t.Errorf("the dry run's review lacks %q", want)
		}
	}
	if strings.Contains(dry, "A backup recipient is set") || regexp.MustCompile(`data-confirm="m-job" disabled`).MatchString(dry) {
		t.Error("a dry run asks for a backup, or cannot start")
	}
	clean := c.change("/jobs/review", url.Values{"action": {"rotate"}, "key": {key}, "no_backup": {"1"}}).Body.String()
	for _, want := range []string{"--no-key-backup", "writes no encrypted backup"} {
		if !strings.Contains(clean, want) {
			t.Errorf("the clean rotation's review lacks %q", want)
		}
	}
	if strings.Contains(clean, "A backup recipient is set") {
		t.Error("a rotation without a backup still checks for a recipient")
	}
}
