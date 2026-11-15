package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// settings validate said "ok" over a .env whose settings were never read: an
// old SSHMGR_ name (dead since #146), a misspelt AUTHAMA_ name, and values
// the setting cannot take (review R-GAP-03).
func TestValidateFlagsNamesAndValuesNothingReads(t *testing.T) {
	env := filepath.Join(t.TempDir(), ".env")
	body := strings.Join([]string{
		"SSHMGR_SNAPSHOT_RETAIN=3",
		"AUTHAMA_SNAPSHOT_RETAINN=3",
		"AUTHAMA_SNAPSHOT_RETAIN=abc",
		"AUTHAMA_AUTO_PIN=maybe",
		"AUTHAMA_HISTORY=0",
		"AUTHAMA_GIT_MODE=ssh",
		"",
	}, "\n")
	if err := os.WriteFile(env, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(DotenvProblems(env), "\n")
	for _, want := range []string{
		"SSHMGR_SNAPSHOT_RETAIN", "AUTHAMA_SNAPSHOT_RETAIN instead",
		"AUTHAMA_SNAPSHOT_RETAINN",
		"AUTHAMA_SNAPSHOT_RETAIN=abc",
		"AUTHAMA_AUTO_PIN=maybe",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("problems do not mention %q:\n%s", want, got)
		}
	}
	for _, fine := range []string{"AUTHAMA_HISTORY", "AUTHAMA_GIT_MODE"} {
		if strings.Contains(got, fine) {
			t.Errorf("a valid %s was flagged:\n%s", fine, got)
		}
	}
}

// Migrating a home renames the old names it copies, so the copy's settings
// still apply; a name already set under its new spelling is left alone.
func TestRenameLegacyNames(t *testing.T) {
	env := filepath.Join(t.TempDir(), ".env")
	body := "# kept\nSSHMGR_SNAPSHOT_RETAIN=3\nSSH_MANAGER_AUTO_PIN=0\nSSHMGR_HISTORY=0\nAUTHAMA_HISTORY=1\nSSHMGR_NOT_A_SETTING=1\n"
	if err := os.WriteFile(env, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	renamed, err := RenameLegacyNames(env)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(env)
	got := string(b)
	for _, want := range []string{"# kept\n", "AUTHAMA_SNAPSHOT_RETAIN=3\n", "AUTHAMA_AUTO_PIN=0\n", "SSHMGR_HISTORY=0\n", "AUTHAMA_HISTORY=1\n", "SSHMGR_NOT_A_SETTING=1\n"} {
		if !strings.Contains(got, want) {
			t.Errorf(".env lacks %q:\n%s", want, got)
		}
	}
	if len(renamed) != 2 {
		t.Errorf("renamed = %v, want the two that map to a setting not already set", renamed)
	}
}
