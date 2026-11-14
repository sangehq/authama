package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Set writes one setting, checked against its kind, and keeps the rest;
// Unset takes it out again, and its section once empty.
func TestSetAndUnsetEditOneSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authama.json")
	if err := Set(path, "web.port", "8000"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "git.mode", "ssh"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "ssh.auto_pin", "false"); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"web.port": "8000", "git.mode": "ssh", "ssh.auto_pin": "0"} {
		if v, ok := FileValue(path, key); !ok || v != want {
			t.Errorf("%s = %q, %v; want %q", key, v, ok, want)
		}
	}
	for _, bad := range [][2]string{{"web.port", "zero"}, {"web.port", "0"}, {"git.mode", "ftp"}, {"ssh.auto_pin", "maybe"}, {"no.such", "x"}} {
		if err := Set(path, bad[0], bad[1]); err == nil {
			t.Errorf("Set(%s, %q) was accepted", bad[0], bad[1])
		}
	}
	if err := Unset(path, "git.mode"); err != nil {
		t.Fatal(err)
	}
	if _, ok := FileValue(path, "git.mode"); ok {
		t.Error("git.mode is still in the file after Unset")
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), `"git"`) {
		t.Errorf("an empty section was left behind:\n%s", b)
	}
	if v, _ := FileValue(path, "web.port"); v != "8000" {
		t.Error("Unset took another setting with it")
	}
}

// The key-recency settings: the window is a whole number of days, 0 or more,
// and the events are a comma-separated list of the events recency knows.
// Each is refused at set time, and validate names a bad value in .env, so a
// typo never silently means the default (R-FEAT-01).
func TestTheKeyRecencySettingsAreChecked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authama.json")
	for key, value := range map[string]string{"keys.new_window_days": "0", "keys.new_events": "created, deployed"} {
		if err := Set(path, key, value); err != nil {
			t.Errorf("Set(%s, %q): %v", key, value, err)
		}
	}
	for _, bad := range [][2]string{
		{"keys.new_window_days", "-1"}, {"keys.new_window_days", "7d"},
		{"keys.new_events", "created,revoked"}, {"keys.new_events", "made"}, {"keys.new_events", ","},
	} {
		if err := Set(path, bad[0], bad[1]); err == nil {
			t.Errorf("Set(%s, %q) was accepted", bad[0], bad[1])
		}
	}
	env := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(env, []byte("AUTHAMA_KEYS_NEW_EVENTS=created,whenever\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(DotenvProblems(env), "\n"); !strings.Contains(got, "whenever") {
		t.Errorf("validate missed a bad event list: %q", got)
	}
}
