package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/browsers"
	"github.com/sangehq/authama/src/internal/util/applog"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// runLogged runs a command as main does, logging it, and returns its error.
func runLogged(t *testing.T, args ...string) error {
	t.Helper()
	root := newRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	return executeLogged(root, args)
}

// Every command is logged, with its flags' names but no secret values, and
// `authama logs` reads it back, filtered, as text or JSON.
func TestCommandsAreLoggedAndReadBack(t *testing.T) {
	withStartup(t)
	p := editHome(t)
	t.Setenv("AUTHAMA_TIMEZONE", "UTC")
	if err := runLogged(t, "ssh", "profile", "add", "work"); err != nil {
		t.Fatal(err)
	}
	_ = runLogged(t, "ssh", "host", "add", "nowhere", "gh", "-H", "github.com")
	logged, err := os.ReadFile(filepath.Join(p.LogDir(), applog.FileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"command":"ssh profile add"`, `"command failed"`} {
		if !strings.Contains(string(logged), want) {
			t.Errorf("the log lacks %s:\n%s", want, logged)
		}
	}
	out := run(t, "logs", "--grep", "profile add")
	if !strings.Contains(out, "ssh profile add") || !strings.Contains(out, "UTC") || strings.Contains(out, "host add") {
		t.Fatalf("logs --grep:\n%s", out)
	}
	if out := run(t, "logs", "--level", "warn", "--json"); !strings.Contains(out, `"level":"WARN"`) || strings.Contains(out, `"level":"INFO"`) {
		t.Fatalf("logs --level warn --json:\n%s", out)
	}
	if _, err := runErr(t, "logs", "--level", "loud"); err == nil {
		t.Fatal("an unknown level was accepted")
	}
	dst := filepath.Join(t.TempDir(), "out.jsonl")
	run(t, "logs", "export", "--out", dst)
	if b, _ := os.ReadFile(dst); !strings.Contains(string(b), "ssh profile add") {
		t.Fatal("export wrote nothing useful")
	}
	if _, err := runErr(t, "logs", "export", "--out", dst); err == nil {
		t.Fatal("export overwrote a file")
	}
	if got := strings.TrimSpace(run(t, "logs", "path")); got != filepath.Join(p.LogDir(), applog.FileName) {
		t.Fatalf("logs path = %q", got)
	}
}

// A secret given on the command line never reaches the log.
func TestTheLogNeverHoldsASecretArgument(t *testing.T) {
	withStartup(t)
	p := editHome(t)
	_ = runLogged(t, "token", "add", "npm", "Super"+"SecretValue")
	_ = runLogged(t, "token", "add", "npm", "--token", "Super"+"SecretValue")
	_ = runLogged(t, "owner", "pin", "set", "8642")
	_ = runLogged(t, "ssh", "profile", "add", "--passphrase=8642", "work")
	b, _ := os.ReadFile(filepath.Join(p.LogDir(), applog.FileName))
	if len(b) == 0 {
		t.Fatal("nothing was logged")
	}
	for _, s := range []string{"SuperSecretValue", "8642"} {
		if strings.Contains(string(b), s) {
			t.Fatalf("the log holds %q:\n%s", s, b)
		}
	}
}

// The log level and retention are settings, set from `logs`.
func TestLogSettingsAreSetFromTheLogsCommand(t *testing.T) {
	p := editHome(t)
	run(t, "logs", "level", "debug")
	run(t, "logs", "retention", "30")
	b, _ := os.ReadFile(p.SettingsFile())
	if !strings.Contains(string(b), `"level": "debug"`) || !strings.Contains(string(b), `"retention_days": 30`) {
		t.Fatalf("authama.json:\n%s", b)
	}
	for _, bad := range [][]string{{"logs", "level", "loud"}, {"logs", "retention", "0"}} {
		if _, err := runErr(t, bad...); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}

// The display zone: set, shown, overridden with --tz, and checked.
func TestTheTimeZoneIsSetShownAndOverridden(t *testing.T) {
	withStartup(t)
	p := editHome(t)
	run(t, "settings", "timezone", "set", "Asia/Tokyo")
	b, _ := os.ReadFile(p.SettingsFile())
	if !strings.Contains(string(b), "Asia/Tokyo") {
		t.Fatalf("authama.json:\n%s", b)
	}
	if out := run(t, "settings", "timezone", "show"); !strings.Contains(out, "Asia/Tokyo") || !strings.Contains(out, "JST") {
		t.Fatalf("show: %s", out)
	}
	if out := run(t, "--tz", "UTC", "settings", "timezone", "show"); !strings.Contains(out, "UTC (now") {
		t.Fatalf("--tz: %s", out)
	}
	if _, err := runErr(t, "--tz", "Mars/Olympus", "settings", "timezone", "show"); err == nil {
		t.Fatal("a bad --tz was accepted")
	}
	if _, err := runErr(t, "settings", "timezone", "set", "Mars/Olympus"); err == nil {
		t.Fatal("a bad zone was saved")
	}
	run(t, "settings", "timezone", "set", "local")
	if b, _ := os.ReadFile(p.SettingsFile()); strings.Contains(string(b), "Asia/Tokyo") {
		t.Fatal("local did not take the setting out")
	}
	if out := run(t, "settings", "timezone", "list", "tokyo"); strings.TrimSpace(out) == "" || !strings.Contains(out, "Asia/Tokyo") {
		t.Fatalf("list: %q", out)
	}
	_ = tz.Set("")
}

// `serve browsers` lists remembered browsers without their secrets, and
// forgets them.
func TestRememberedBrowsersAreListedAndForgotten(t *testing.T) {
	p := editHome(t)
	state := paths.Paths{SSHDir: p.SSHDir, ConfigDir: p.ConfigDir}.StateDir()
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	file := browsers.File(state)
	id, sec, err := browsers.Remember(file, "Firefox on Linux", time.Now(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	out := run(t, "serve", "browsers")
	if !strings.Contains(out, id) || !strings.Contains(out, "Firefox on Linux") || strings.Contains(out, sec) {
		t.Fatalf("serve browsers:\n%s", out)
	}
	if out := run(t, "serve", "browsers", "--json"); strings.Contains(out, "hash") || strings.Contains(out, sec) {
		t.Fatalf("--json shows a secret or hash:\n%s", out)
	}
	if _, err := runErr(t, "serve", "browsers", "forget"); err == nil {
		t.Fatal("forget with nothing named")
	}
	if _, err := runErr(t, "serve", "browsers", "forget", "nope"); err == nil {
		t.Fatal("forgetting an unknown id said nothing")
	}
	run(t, "serve", "browsers", "forget", id)
	if browsers.Known(file, id, time.Now()) {
		t.Fatal("still remembered")
	}
}
