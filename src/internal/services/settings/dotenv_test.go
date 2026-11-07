package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseDotenvHandlesTheCommonForms(t *testing.T) {
	in := "\ufeff# a comment\n" +
		"\n" +
		"PLAIN=value\n" +
		"export EXPORTED=yes\n" +
		`DOUBLE="landlord panel api"` + "\n" +
		`ESCAPED="a\"b\nc"` + "\n" +
		`SINGLE='literal $HOME \n'` + "\n" +
		"BARE=word # trailing comment\n" +
		"HASH=a#b\n" +
		"  SPACED  =  padded  \n" +
		"EMPTY=\n" +
		"EMPTYNOTE=     # just a note\n" +
		"LITERALHASH=#x\n" +
		"not an assignment\n" +
		"1BAD=x\n" +
		`UNCLOSED="oops` + "\n" +
		`JUNK="x" y` + "\n"
	pairs, warns := ParseDotenv(strings.NewReader(in))
	got := map[string]string{}
	for _, p := range pairs {
		got[p.Key] = p.Value
	}
	want := map[string]string{
		"PLAIN": "value", "EXPORTED": "yes", "DOUBLE": "landlord panel api",
		"ESCAPED": "a\"b\nc", "SINGLE": `literal $HOME \n`, "BARE": "word", "HASH": "a#b",
		"SPACED": "padded", "EMPTY": "", "EMPTYNOTE": "", "LITERALHASH": "#x",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	for _, k := range []string{"1BAD", "UNCLOSED", "JUNK"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s should have been rejected", k)
		}
	}
	if len(warns) != 4 {
		t.Errorf("want 4 warnings (the four bad lines), got %d: %v", len(warns), warns)
	}
	for _, w := range warns {
		if strings.Contains(w, "oops") || strings.Contains(w, "x\" y") {
			t.Errorf("a warning repeats the line's value, which may be a secret: %q", w)
		}
	}
}

func fakeEnv(m map[string]string) Env {
	return Env{
		Lookup: func(k string) (string, bool) { v, ok := m[k]; return v, ok },
		Setenv: func(k, v string) error { m[k] = v; return nil },
	}
}

func TestLoadDotenvNeverOverridesAndIgnoresAMissingFile(t *testing.T) {
	dir := t.TempDir()
	res := LoadDotenv(filepath.Join(dir, ".env"), fakeEnv(map[string]string{}))
	if res.Present || len(res.Warnings) != 0 || res.Refused != "" {
		t.Errorf("a missing file is not an event: %+v", res)
	}
	path := filepath.Join(dir, ".env")
	body := "GH_TOKEN=file\nNEW=file\nAUTHAMA_SNAPSHOT_RETAIN=4\nAUTHAMA_CONFIG_DIR=/x\nSSH_MANAGER_HOME=\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"GH_TOKEN": "real", "SSH_MANAGER_SNAPSHOT_RETAIN": "9"}
	res = LoadDotenv(path, fakeEnv(env))
	if env["GH_TOKEN"] != "real" || env["NEW"] != "file" {
		t.Errorf("env after load: %v", env)
	}
	// SSH_MANAGER_SNAPSHOT_RETAIN is no longer read, so it does not stop the
	// file setting the current name.
	if env["AUTHAMA_SNAPSHOT_RETAIN"] != "4" {
		t.Errorf("the file should set AUTHAMA_SNAPSHOT_RETAIN; an old name in the environment is ignored: %v", env)
	}
	if _, ok := env["AUTHAMA_CONFIG_DIR"]; ok {
		t.Error("the file moved the config home")
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "AUTHAMA_CONFIG_DIR") {
		t.Errorf("want one warning about AUTHAMA_CONFIG_DIR (the empty template line is fine), got %v", res.Warnings)
	}
}

func TestLoadDotenvRefusesAFileOthersCanRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no mode bits on Windows")
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("GH_TOKEN=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	res := LoadDotenv(path, fakeEnv(env))
	if res.Refused == "" || len(env) != 0 {
		t.Errorf("a group-readable .env was loaded: %+v %v", res, env)
	}
}
