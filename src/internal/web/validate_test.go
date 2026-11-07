package web

import (
	"os"
	"strings"
	"testing"
)

// /validate runs every validate command's check on one page and names each
// command, so the page and the CLI cannot be read as different checks.
func TestTheValidatePageRunsEveryCheck(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	res := c.get("/validate")
	if res.Code != 200 {
		t.Fatalf("GET /validate = %d", res.Code)
	}
	body := res.Body.String()
	for _, want := range []string{
		`data-validate-section="manifest"`, `data-validate-section="settings"`,
		`data-validate-section="keys"`, `data-validate-section="sshconfig"`,
		"settings validate", "ssh validate", "ssh config check",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/validate lacks %q", want)
		}
	}
}

// A broken settings file fails its section, with the schema's own words, and
// the page still answers: one bad file does not hide the other checks.
func TestABadSettingsFileFailsItsSection(t *testing.T) {
	s, _, _ := newTestServer(t)
	p := s.opts.Engine.Home()
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.SettingsFile(), []byte(`{"no_such_setting": true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	v := validateView(p)
	var got *ValidateSection
	for i := range v.Sections {
		if v.Sections[i].ID == "settings" {
			got = &v.Sections[i]
		}
	}
	if got == nil || got.OK || len(got.Problems) == 0 {
		t.Fatalf("settings section = %+v, want a failure", got)
	}
	if v.Failed == 0 {
		t.Fatal("Failed counts no section")
	}
	if len(v.Sections) != 4 {
		t.Fatalf("a bad settings file cut the page to %d sections", len(v.Sections))
	}
	c := signIn(t, s)
	if body := c.get("/validate").Body.String(); !strings.Contains(body, `data-validate-section="settings" data-ok="false"`) {
		t.Error("the page does not mark the settings section failed")
	}
}

// A manifest that does not load fails its section, and the two checks that
// need it say why they did not run rather than reporting a clean result.
func TestAnUnreadableManifestIsNotReportedClean(t *testing.T) {
	s, _, _ := newTestServer(t)
	p := s.opts.Engine.Home()
	if err := os.WriteFile(p.Manifest(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := validateView(p)
	for _, sec := range v.Sections {
		switch sec.ID {
		case "manifest", "keys", "sshconfig":
			if sec.OK {
				t.Errorf("%s reported ok with an unreadable manifest", sec.ID)
			}
		}
	}
}
