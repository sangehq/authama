package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// `export <list>` as CSV is the list's own --csv: the same pipeline, the same
// columns, byte for byte.
func TestExportIsTheListsCSV(t *testing.T) {
	editHome(t)
	run(t, "profile", "add", "work")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "--tag", "prd", "--tag", "web")
	run(t, "host", "add", "work", "db1", "-H", "db1.example", "-u", "deploy")
	for _, c := range [][2][]string{
		{{"export", "hosts"}, {"ssh", "list", "--csv"}},
		{{"export", "keys"}, {"ssh", "key", "list", "--csv"}},
	} {
		if got, want := run(t, c[0]...), run(t, c[1]...); got != want {
			t.Errorf("%v differs from %v:\n--- export\n%s--- csv\n%s", c[0], c[1], got, want)
		}
	}
}

func TestExportFiltersChoosesColumnsSortsAndFormats(t *testing.T) {
	editHome(t)
	run(t, "profile", "add", "work")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "--tag", "prd", "--tag", "web")
	run(t, "host", "add", "work", "db1", "-H", "db1.example", "-u", "deploy")

	out := run(t, "export", "hosts", "--tag", "web", "--columns", "alias,hostname=host,key.type", "--format", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0]["alias"] != "web1" || rows[0]["host"] != "web1.example" || rows[0]["key.type"] == "" {
		t.Fatalf("rows = %v", rows)
	}
	if out := run(t, "export", "hosts", "--columns", "alias", "--sort", "-alias", "--no-header"); out != "web1\ndb1\n" {
		t.Errorf("sorted, no header: %q", out)
	}
	if out := run(t, "export", "hosts", "--columns", "alias", "--sort", "alias", "--limit", "1", "--no-header"); out != "db1\n" {
		t.Errorf("limit 1: %q", out)
	}
	if out := run(t, "export", "hosts", "--fields"); !strings.Contains(out, "key.fingerprint\n") || !strings.Contains(out, "alias\n") {
		t.Errorf("--fields:\n%s", out)
	}
	if out := run(t, "export", "hosts", "--format", "php", "--compact"); !strings.HasPrefix(out, "<?php\n\nreturn [[") {
		t.Errorf("php:\n%s", out)
	}
}

// A selection saved as JSON drives it, and --out writes a private file.
func TestExportTakesASavedSelectionAndWritesAFile(t *testing.T) {
	editHome(t)
	run(t, "profile", "add", "work")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy")
	dir := t.TempDir()
	sel := filepath.Join(dir, "sel.json")
	if err := os.WriteFile(sel, []byte(`{"version":1,"grain":"host","columns":[{"field":"alias"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "hosts.yaml")
	run(t, "export", "--selection", sel, "--format", "yaml", "--out", target)
	b, err := os.ReadFile(target)
	if err != nil || string(b) != "- alias: web1\n" {
		t.Fatalf("file = %q, %v", b, err)
	}
	// Windows keeps no Unix mode bits: an ACL guards the file there instead.
	if fi, _ := os.Stat(target); runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("mode %v is not private", fi.Mode().Perm())
	}
}

func TestExportSaysWhatIsWrong(t *testing.T) {
	editHome(t)
	for args, want := range map[string]string{
		"export":                          "name a list",
		"export planets":                  `no list or saved selection named "planets"`,
		"export hosts --format xlsx":      "unknown format",
		"export hosts --columns nope":     "no field",
		"export hosts --selection x.json": "not both",
	} {
		if _, err := runErr(t, strings.Fields(args)...); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", args, err, want)
		}
	}
}

// Transforms from the flags: a template column, a dedupe, a group with
// aggregates and totals, a step as JSON; and --fields lists what they wrote.
func TestExportTransforms(t *testing.T) {
	editHome(t)
	run(t, "profile", "add", "work")
	run(t, "profile", "add", "home")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "--tag", "prd")
	run(t, "host", "add", "work", "db1", "-H", "db1.example", "-u", "deploy")
	run(t, "host", "add", "home", "nas", "-H", "nas.lan", "-u", "me")

	if out := run(t, "export", "hosts", "--template", "ssh=ssh {user}@{hostname}", "--columns", "alias,ssh", "--sort", "alias", "--no-header"); out != "db1,ssh deploy@db1.example\nnas,ssh me@nas.lan\nweb1,ssh deploy@web1.example\n" {
		t.Errorf("template:\n%s", out)
	}
	if out := run(t, "export", "hosts", "--dedupe", "profile", "--columns", "profile", "--sort", "profile", "--no-header"); out != "home\nwork\n" {
		t.Errorf("dedupe:\n%s", out)
	}
	if out := run(t, "export", "hosts", "--group-by", "profile", "--aggregate", "count,list:alias=aliases", "--totals", "--sort", "profile"); out != "profile,count,aliases\nhome,1,nas\nwork,2,web1 db1\ntotal,3,web1 db1 nas\n" {
		t.Errorf("group:\n%s", out)
	}
	if out := run(t, "export", "hosts", "--transform", `{"kind":"format","field":"alias","format":"upper"}`, "--columns", "alias", "--sort", "alias", "--no-header"); out != "DB1\nNAS\nWEB1\n" {
		t.Errorf("--transform:\n%s", out)
	}
	if out := run(t, "export", "hosts", "--template", "where={profile}/{alias}", "--fields"); !strings.HasSuffix(out, "\nwhere\n") {
		t.Errorf("--fields after a transform:\n%s", out)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--aggregate", "count"}, "need --group-by"},
		{[]string{"--template", "nope"}, "give as=template"},
		{[]string{"--transform", `{"kind":"map","expr":"x"}`}, "unknown field"},
		{[]string{"--group-by", "profile", "--aggregate", "avg:port"}, `no aggregate "avg"`},
		{[]string{"--dedupe", "profile", "--format", "ssh_config"}, "takes no transforms"},
	} {
		if _, err := runErr(t, append([]string{"export", "hosts"}, c.args...)...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}

// A selection saved by name: in either store, listed, shown, exported by its
// name or by --selection, the user's own winning, and deleted.
func TestExportSelectionsSavedByName(t *testing.T) {
	p := editHome(t)
	run(t, "profile", "add", "work")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "--tag", "prd")
	run(t, "host", "add", "work", "db1", "-H", "db1.example", "-u", "deploy")
	dir := t.TempDir()
	write := func(name, body string) string {
		f := filepath.Join(dir, name)
		if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return f
	}
	webOnly := write("web.json", `{"version":1,"grain":"host","where":{"tag":["prd"]},"columns":[{"field":"alias"}]}`)
	dbOnly := write("db.json", `{"version":1,"grain":"host","where":{"exclude":"web"},"columns":[{"field":"alias"}]}`)

	if out := run(t, "export", "selections", "save", "mine", "--from", webOnly, "--to", "manifest"); !strings.Contains(out, "in the manifest store") {
		t.Errorf("save: %s", out)
	}
	if out := run(t, "export", "mine", "--no-header"); out != "web1\n" {
		t.Errorf("export by name: %q", out)
	}
	// The setting sends a save to the manifest; --to overrides it.
	t.Setenv("AUTHAMA_EXPORT_SAVE_TO", "manifest")
	run(t, "export", "selections", "save", "shared", "--from", dbOnly)
	run(t, "export", "selections", "save", "mine", "--from", dbOnly, "--to", "user")
	if out := run(t, "export", "--selection", "mine", "--no-header"); out != "db1\n" {
		t.Errorf("the user's own should win: %q", out)
	}
	b, _ := os.ReadFile(p.Manifest())
	if !strings.Contains(string(b), `"selections"`) || !strings.Contains(string(b), `"shared"`) {
		t.Errorf("the manifest holds no selections:\n%s", b)
	}
	list := run(t, "export", "selections", "list")
	for _, want := range []string{"mine    user", "mine    manifest  host  hidden by your own", "shared  manifest"} {
		if !strings.Contains(list, want) {
			t.Errorf("list lacks %q:\n%s", want, list)
		}
	}
	if out := run(t, "export", "selections", "show", "shared"); !strings.Contains(out, `"exclude": "web"`) {
		t.Errorf("show:\n%s", out)
	}
	if _, err := runErr(t, "export", "selections", "delete", "mine"); err == nil || !strings.Contains(err.Error(), "both stores") {
		t.Errorf("delete from both: %v", err)
	}
	run(t, "export", "selections", "delete", "mine", "--from", "user")
	if out := run(t, "export", "mine", "--no-header"); out != "web1\n" {
		t.Errorf("after deleting the user's: %q", out)
	}
	run(t, "export", "selections", "delete", "mine")
	run(t, "export", "selections", "delete", "shared")
	if b, _ := os.ReadFile(p.Manifest()); strings.Contains(string(b), `"export"`) {
		t.Errorf("an empty export section was left:\n%s", b)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"export", "nothing-here"}, "no list or saved selection"},
		{[]string{"export", "selections", "save", "hosts", "--from", webOnly}, "built-in list"},
		{[]string{"export", "selections", "save", "Bad", "--from", webOnly}, "selection name"},
		{[]string{"export", "selections", "save", "x", "--from", webOnly, "--to", "cloud"}, `no store "cloud"`},
		{[]string{"export", "selections", "save", "x"}, "give --from"},
		{[]string{"export", "selections", "delete", "gone"}, "no saved selection"},
	} {
		if _, err := runErr(t, c.args...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}
