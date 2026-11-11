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
