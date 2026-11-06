package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAcceptsAFullFile(t *testing.T) {
	f, err := Parse("authama.json", []byte(`{
  "$schema": "`+SchemaURL+`",
  "version": 1,
  "paths": {"state_dir": "~/state"},
  "backup": {"age_recipient": "age1x", "snapshot_retain": 3, "old_key_max_age_days": 0},
  "ssh": {"auto_pin": false},
  "git": {"mode": "https"},
  "tokens": {"docker_creds_store": "refuse"},
  "history": {"enabled": true},
  "notifications": {"desktop": false}
}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"backup.snapshot_retain": "3", "backup.old_key_max_age_days": "0", "ssh.auto_pin": "0",
		"git.mode": "https", "history.enabled": "1", "notifications.desktop": "0", "paths.state_dir": "~/state",
	}
	for k, v := range want {
		if f.Values[k] != v {
			t.Errorf("%s = %q, want %q", k, f.Values[k], v)
		}
	}
	env := map[string]string{"AUTHAMA_GIT_MODE": "ssh"}
	res := f.Apply(fakeEnv(env))
	if env["AUTHAMA_GIT_MODE"] != "ssh" {
		t.Error("authama.json overrode the environment")
	}
	if env["AUTHAMA_SNAPSHOT_RETAIN"] != "3" || env["AUTHAMA_AUTO_PIN"] != "0" {
		t.Errorf("not applied: %v", env)
	}
	if !strings.HasSuffix(filepath.ToSlash(env["AUTHAMA_STATE_DIR"]), "/state") || strings.HasPrefix(env["AUTHAMA_STATE_DIR"], "~") {
		t.Errorf("state_dir not expanded: %q", env["AUTHAMA_STATE_DIR"])
	}
	if len(res.Kept) != 1 || res.Kept[0] != "AUTHAMA_GIT_MODE" {
		t.Errorf("Kept = %v", res.Kept)
	}
}

func TestParseListsEveryProblem(t *testing.T) {
	_, err := Parse("a.json", []byte(`{"version": 2, "bogus": {}, "backup": {"snapshot_retain": 0, "nope": 1},
		"git": {"mode": "ftp"}, "ssh": {"auto_pin": "yes"}}`))
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want a ValidationError, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"version", "bogus", "backup.snapshot_retain", "backup.nope", "git.mode", "ssh.auto_pin", "a.json"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error does not mention %s:\n%s", want, msg)
		}
	}
	if _, err := Parse("b.json", []byte(`{not json`)); err == nil {
		t.Error("malformed JSON passed")
	}
}

func TestAMissingSettingsFileIsEmpty(t *testing.T) {
	f, err := LoadFile(filepath.Join(t.TempDir(), "authama.json"))
	if err != nil || (f != nil && len(f.Values) != 0) {
		t.Errorf("missing file: %v %v", f, err)
	}
}

func TestTheStarterFileValidates(t *testing.T) {
	if _, err := Parse("starter", Starter()); err != nil {
		t.Fatal(err)
	}
}

// docs/schema/authama.schema.json is what editors validate against (the
// $schema URL points at it on main), so it must match the Specs exactly.
// UPDATE_SETTINGS_SCHEMA=1 rewrites it.
func TestSchemaIsCurrent(t *testing.T) {
	got, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("the schema is not JSON: %v", err)
	}
	path := filepath.Join("..", "..", "..", "..", "docs", "schema", "authama.schema.json")
	if os.Getenv("UPDATE_SETTINGS_SCHEMA") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with UPDATE_SETTINGS_SCHEMA=1)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
		t.Error("docs/schema/authama.schema.json is stale; run with UPDATE_SETTINGS_SCHEMA=1")
	}
}
