package applog

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configure(t *testing.T, level string) string {
	t.Helper()
	dir := t.TempDir()
	Configure(Config{Dir: dir, Level: ParseLevel(level), RetentionDays: 1, MaxSizeMB: 1})
	t.Cleanup(func() { Configure(Config{}) })
	return dir
}

// fake builds a token-shaped value at run time, so no literal in this file
// looks like a credential to a scanner: prefix, then n of c.
func fake(prefix string, c byte, n int) string { return prefix + strings.Repeat(string(c), n) }

func TestSecretsNeverReachTheLog(t *testing.T) {
	dir := configure(t, "debug")
	gh := fake("gh"+"p_", 'a', 36)
	pat := fake("github"+"_pat_", 'B', 30)
	gl := fake("gl"+"pat-", 'c', 22)
	slack := fake("xo"+"xb-", '1', 20)
	age := fake("AGE-SECRET-"+"KEY-1", 'Q', 40)
	pass, pin, link := "s3cr3t-passphrase", "4321", "onetimelinktoken99"
	block := "-----BEGIN OPENSSH " + "PRIVATE KEY-----\nAAAA\n-----END OPENSSH " + "PRIVATE KEY-----"
	Info("sign in", "url", "http://127.0.0.1:7822/?token="+link+"&x=1", "passphrase", pass, "pin", pin)
	Warn("bearer", "header", "Bearer "+gh)
	Info("command", "args", Args([]string{"--passphrase", pass, "--pin=" + pin, "--name", "work"}))
	Error("failed", "error", errors.New("bad token "+pat))
	Info("key", "body", block)
	Debug("misc", "detail", gl+" "+slack+" "+age)
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{gh, pat, gl, slack, age, pass, pin, link, "AAAA"} {
		if strings.Contains(string(b), s) {
			t.Errorf("the log holds %q:\n%s", s, b)
		}
	}
	if !strings.Contains(string(b), "work") || !strings.Contains(string(b), Redacted) {
		t.Fatalf("redaction removed too much or nothing:\n%s", b)
	}
	if fi, _ := os.Stat(filepath.Join(dir, FileName)); fi.Mode().Perm() != 0o600 {
		t.Fatalf("authama.log is %v", fi.Mode().Perm())
	}
}

func TestTheLevelFiltersWhatIsWritten(t *testing.T) {
	dir := configure(t, "warn")
	Debug("d")
	Info("i")
	Warn("w")
	Error("e")
	all, err := Read(dir, Filter{Level: slog.LevelDebug})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Msg != "w" || all[1].Msg != "e" {
		t.Fatalf("%+v", all)
	}
	for _, l := range []string{"debug", "INFO", "warning", "error"} {
		if !ValidLevel(l) {
			t.Fatalf("%s is a level", l)
		}
	}
	if ValidLevel("loud") {
		t.Fatal("loud is not a level")
	}
}

func TestRecordsAreReadFilteredAndFollowed(t *testing.T) {
	dir := configure(t, "debug")
	Info("serve started", "area", "web")
	Warn("signin.refused", "area", "auth")
	Info("command", "command", "ssh host add")
	got, _ := Read(dir, Filter{Level: slog.LevelWarn})
	if len(got) != 1 || got[0].Msg != "signin.refused" {
		t.Fatalf("level: %+v", got)
	}
	got, _ = Read(dir, Filter{Level: slog.LevelDebug, Grep: "HOST ADD"})
	if len(got) != 1 || got[0].Attrs["command"] != "ssh host add" {
		t.Fatalf("grep: %+v", got)
	}
	got, _ = Read(dir, Filter{Level: slog.LevelDebug, Since: time.Now().Add(time.Hour)})
	if len(got) != 0 {
		t.Fatalf("since: %+v", got)
	}
	got, _ = Read(dir, Filter{Level: slog.LevelDebug, Limit: 2})
	if len(got) != 2 || got[1].Msg != "command" {
		t.Fatalf("limit keeps the newest: %+v", got)
	}
	off := Size(dir)
	Info("later")
	more, next, err := Tail(dir, off, Filter{Level: slog.LevelDebug})
	if err != nil || len(more) != 1 || more[0].Msg != "later" || next != Size(dir) {
		t.Fatalf("tail: %+v %d %v", more, next, err)
	}
	if e, ok := Parse("not json"); ok {
		t.Fatalf("parsed %+v", e)
	}
}

func TestTheLogRotatesAndOldFilesArePruned(t *testing.T) {
	dir := configure(t, "info")
	big := strings.Repeat("x", 300<<10)
	for i := 0; i < 5; i++ {
		Info("big", "detail", big)
	}
	files := Files(dir)
	if len(files) < 2 || filepath.Base(files[len(files)-1]) != FileName {
		t.Fatalf("no rotation: %v", files)
	}
	old := files[0]
	past := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if n := Prune(dir, 1, time.Now()); n != 1 {
		t.Fatalf("pruned %d", n)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("an expired rotated log is still there")
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatal("prune removed the live log")
	}
}
