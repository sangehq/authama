package applog

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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
	if fi, _ := os.Stat(filepath.Join(dir, FileName)); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
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

// More secret shapes, and a secret nested in a map, are blanked (S-05).
func TestMoreSecretShapesNeverReachTheLog(t *testing.T) {
	dir := configure(t, "debug")
	pw := fake("pw", 'z', 12)
	q := fake("qv", 'y', 16)
	npm := fake("np"+"m_", 'n', 36)
	aws := fake("AK"+"IA", 'W', 16)
	nested := fake("nv", 'x', 14)
	Info("url", "remote", "https://ada:"+pw+"@git.example/x.git", "call", "https://api.example/v1?access_token="+q+"&page=2")
	Info("tokens", "detail", npm+" "+aws)
	Info("nested", "cfg", map[string]any{"name": "work", "inner": map[string]any{"password": nested}})
	b, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{pw, q, npm, aws, nested} {
		if strings.Contains(string(b), s) {
			t.Errorf("the log holds %q:\n%s", s, b)
		}
	}
	for _, s := range []string{"ada:", "git.example", "page=2", "work"} {
		if !strings.Contains(string(b), s) {
			t.Errorf("redaction removed %q:\n%s", s, b)
		}
	}
}

// A record too long to read back is skipped, and the records after it in the
// same file are still read (B-13); a limit reads the newest records (S-07).
func TestAnOverlongLineHidesNothingAfterIt(t *testing.T) {
	dir := t.TempDir()
	line := func(msg string) string {
		return `{"time":"2026-11-23T00:00:00Z","level":"INFO","msg":"` + msg + `"}` + "\n"
	}
	body := line("first") + `{"msg":"` + strings.Repeat("x", 2<<20) + `"}` + "\n" + line("after") + line("last")
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, end, err := ReadFollow(dir, Filter{})
	if err != nil || len(got) != 3 || got[1].Msg != "after" || end != int64(len(body)) {
		t.Fatalf("read %d records %v, end %d of %d", len(got), err, end, len(body))
	}
	got, _ = Read(dir, Filter{Limit: 1})
	if len(got) != 1 || got[0].Msg != "last" {
		t.Fatalf("limit 1: %+v", got)
	}
}

// Retention is kept when logging starts, not only on rotation (B-08); a log
// left world-readable is made owner-only again (S-07).
func TestConfigurePrunesAndTightensModes(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "authama-20200101T000000.000.log")
	if err := os.WriteFile(old, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, FileName)
	if err := os.WriteFile(live, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	Configure(Config{Dir: dir, Level: slog.LevelInfo, RetentionDays: 1, MaxSizeMB: 1})
	t.Cleanup(func() { Configure(Config{}) })
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("an expired rotated log survived Configure")
	}
	Info("x")
	if fi, _ := os.Stat(live); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("authama.log is %v after a write", fi.Mode().Perm())
	}
}
