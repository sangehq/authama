package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/paths"
)

// dotenvRecipient is a stand-in age recipient, not a key. It is a constant so
// the line that names the variable carries no literal value for secret
// scanners to flag.
const dotenvRecipient = "age1fromdotenv"

// withStartup runs start-up for one test, against a home made for it: HOME and
// USERPROFILE point at a temp dir, every setting variable is cleared (and
// restored afterwards), and the XDG bases are unset so the defaults apply.
func withStartup(t *testing.T) (home string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	for _, n := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME",
		"AUTHAMA_CONFIG_DIR", "AUTHAMA_STATE_DIR", "AUTHAMA_NO_DOTENV", "AUTHAMA_DEV_ROOT",
		"SSH_MANAGER_HOME", "SSH_MANAGER_CONFIG_DIR", "SSHMGR_DEV_ROOT",
		"AUTHAMA_SNAPSHOT_RETAIN", "SSH_MANAGER_SNAPSHOT_RETAIN",
		"AUTHAMA_AGE_RECIPIENT", "SSH_MANAGER_AGE_RECIPIENT",
		"AUTHAMA_AUTO_PIN", "SSH_MANAGER_AUTO_PIN", "AUTHAMA_GIT_MODE", "SSHMGR_GIT_MODE",
		"AUTHAMA_TEST_TOKEN"} {
		t.Setenv(n, "")
		_ = os.Unsetenv(n)
	}
	releaseHeldLock(t)
	runStartup = true
	t.Cleanup(func() { runStartup = false })
	return home
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// The order the docs give: flags > real env > .env > authama.json > defaults.
func TestStartupLayersTheSettingsInTheDocumentedOrder(t *testing.T) {
	home := withStartup(t)
	cfg := filepath.Join(home, ".config", "authama")
	writeFile(t, filepath.Join(cfg, ".env"), strings.Join([]string{
		"# comment",
		`export AUTHAMA_SNAPSHOT_RETAIN="7"`,
		"AUTHAMA_AGE_RECIPIENT='" + dotenvRecipient + "' # trailing",
		"AUTHAMA_TEST_TOKEN=from-dotenv",
		"this line is not an assignment",
		"",
	}, "\n"), 0o600)
	writeFile(t, filepath.Join(cfg, "authama.json"), `{
  "version": 1,
  "backup": {"snapshot_retain": 3, "age_recipient": "age1fromjson", "old_key_max_age_days": 30},
  "ssh": {"auto_pin": false}
}`, 0o600)
	t.Setenv("AUTHAMA_TEST_TOKEN", "from-env")

	out, err := runErr(t, "settings", "show", "--effective")
	if err != nil {
		t.Fatalf("settings show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "line 5") {
		t.Errorf("the malformed .env line was not warned about by number:\n%s", out)
	}
	if os.Getenv("AUTHAMA_TEST_TOKEN") != "from-env" {
		t.Error(".env overrode a variable already in the environment")
	}
	if got := os.Getenv("AUTHAMA_SNAPSHOT_RETAIN"); got != "7" {
		t.Errorf("AUTHAMA_SNAPSHOT_RETAIN = %q, want .env's 7 over authama.json's 3", got)
	}
	if got := os.Getenv("AUTHAMA_AGE_RECIPIENT"); got != dotenvRecipient {
		t.Errorf("AUTHAMA_AGE_RECIPIENT = %q, want the single-quoted .env value", got)
	}
	if got := os.Getenv("AUTHAMA_OLD_KEY_MAX_AGE_DAYS"); got != "30" {
		t.Errorf("authama.json did not fill a gap: %q", got)
	}
	if got := os.Getenv("AUTHAMA_AUTO_PIN"); got != "0" {
		t.Errorf("auto_pin false should apply as 0, got %q", got)
	}
	for _, want := range []string{".env", "authama.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("--effective does not name the source %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "from-dotenv") || strings.Contains(out, "from-env") {
		t.Error("a non-setting .env value was printed")
	}
}

// A legacy name set in the real environment still beats the files' current
// name, and earns a deprecation notice.
func TestStartupHonoursLegacyNamesOverTheFiles(t *testing.T) {
	home := withStartup(t)
	writeFile(t, filepath.Join(home, ".config", "authama", ".env"), "AUTHAMA_SNAPSHOT_RETAIN=7\n", 0o600)
	t.Setenv("SSH_MANAGER_SNAPSHOT_RETAIN", "5")
	out, err := runErr(t, "settings", "path")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if os.Getenv("AUTHAMA_SNAPSHOT_RETAIN") != "" {
		t.Error(".env set the current name while its legacy spelling was in the environment")
	}
	if !strings.Contains(out, "SSH_MANAGER_SNAPSHOT_RETAIN -> AUTHAMA_SNAPSHOT_RETAIN") {
		t.Errorf("no deprecation notice:\n%s", out)
	}
}

func TestStartupSkipsDotenvWhenAskedTo(t *testing.T) {
	for _, how := range []string{"flag", "env"} {
		t.Run(how, func(t *testing.T) {
			home := withStartup(t)
			writeFile(t, filepath.Join(home, ".config", "authama", ".env"), "AUTHAMA_TEST_TOKEN=x\n", 0o600)
			args := []string{"settings", "path"}
			if how == "flag" {
				args = append([]string{"--no-dotenv"}, args...)
			} else {
				t.Setenv("AUTHAMA_NO_DOTENV", "1")
			}
			if out, err := runErr(t, args...); err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			if _, ok := os.LookupEnv("AUTHAMA_TEST_TOKEN"); ok {
				t.Error(".env was read although it was switched off")
			}
		})
	}
}

// A .env anyone else can read is refused, not loaded.
func TestStartupRefusesAGroupReadableDotenv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no group/other mode bits")
	}
	home := withStartup(t)
	writeFile(t, filepath.Join(home, ".config", "authama", ".env"), "AUTHAMA_TEST_TOKEN=x\n", 0o644)
	out, err := runErr(t, "settings", "path")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, ok := os.LookupEnv("AUTHAMA_TEST_TOKEN"); ok {
		t.Error("a group-readable .env was loaded")
	}
	if !strings.Contains(out, "chmod 600") {
		t.Errorf("the refusal does not say how to fix it:\n%s", out)
	}
}

// A missing .env and authama.json are the normal case: nothing is printed.
func TestStartupIsQuietWithoutFiles(t *testing.T) {
	withStartup(t)
	out, err := runErr(t, "settings", "path")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "warning") || strings.Contains(out, "refus") {
		t.Errorf("start-up complained about files that do not exist:\n%s", out)
	}
}

// An invalid authama.json stops ordinary commands with every problem listed,
// while `settings validate` still runs to diagnose it.
func TestStartupStopsOnAnInvalidSettingsFile(t *testing.T) {
	home := withStartup(t)
	writeFile(t, filepath.Join(home, ".config", "authama", "authama.json"),
		`{"version": 1, "backup": {"snapshot_retain": 0, "typo": 1}, "git": {"mode": "nope"}}`, 0o600)
	out, err := runErr(t, "ssh", "list")
	if err == nil {
		t.Fatalf("an invalid authama.json did not stop the command:\n%s", out)
	}
	for _, want := range []string{"backup.snapshot_retain", "backup.typo", "git.mode"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %s: %v", want, err)
		}
	}
	out, err = runErr(t, "settings", "validate")
	if err == nil {
		t.Errorf("settings validate passed an invalid file:\n%s", out)
	}
	if !strings.Contains(out, "git.mode") {
		t.Errorf("settings validate does not list the problems:\n%s", out)
	}
}

// First run under the new name copies the 0.1.0 home, leaves it in place with
// a marker, and moves the state into the state home - once.
func TestStartupMigratesTheOldHomeOnce(t *testing.T) {
	home := withStartup(t)
	old := filepath.Join(home, ".config", "ssh-manager")
	writeFile(t, filepath.Join(old, "manifest.json"), `{"version":1,"profiles":{}}`, 0o600)
	writeFile(t, filepath.Join(old, ".env"), "AUTHAMA_TEST_TOKEN=x\n", 0o600)
	writeFile(t, filepath.Join(old, "log", "audit.log"), "", 0o600)

	out, err := runErr(t, "settings", "path")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "first run under the new name") {
		t.Errorf("no migration notice:\n%s", out)
	}
	cfg := filepath.Join(home, ".config", "authama")
	for _, f := range []string{"manifest.json", ".env"} {
		if _, err := os.Stat(filepath.Join(cfg, f)); err != nil {
			t.Errorf("%s was not copied: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(old, "manifest.json")); err != nil {
		t.Error("the old home was not kept")
	}
	if _, err := os.Stat(filepath.Join(old, paths.MigratedMarker)); err != nil {
		t.Error("the old home carries no marker")
	}
	state := filepath.Join(home, ".local", "state", "authama")
	if _, err := os.Stat(filepath.Join(state, "log", "audit.log")); err != nil {
		t.Errorf("the log did not move to the state home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "log")); err == nil {
		t.Error("the log is still in the config home")
	}
	if os.Getenv("AUTHAMA_TEST_TOKEN") != "x" {
		t.Error("the migrated .env was not loaded on the same run")
	}

	out, err = runErr(t, "settings", "path")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "first run") || strings.Contains(out, "moved") {
		t.Errorf("the second run migrated again:\n%s", out)
	}
}

// Help and version never touch the home.
func TestStartupLeavesHelpAndVersionAlone(t *testing.T) {
	home := withStartup(t)
	old := filepath.Join(home, ".config", "ssh-manager")
	writeFile(t, filepath.Join(old, "manifest.json"), `{}`, 0o600)
	for _, args := range [][]string{{"version"}, {"--help"}, {"help", "settings"}} {
		if out, err := runErr(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "authama")); err == nil {
		t.Error("help/version migrated the home")
	}
}
