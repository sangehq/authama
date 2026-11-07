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
	if out := run(t, "export", "hosts", "--pivot", "profile:user", "--totals", "--sort", "profile"); out != "profile,deploy,me,total\nhome,0,1,1\nwork,2,0,2\ntotal,2,1,3\n" {
		t.Errorf("pivot:\n%s", out)
	}
	if out := run(t, "export", "hosts", "--template", "where={profile}/{alias}", "--fields"); !strings.HasSuffix(out, "\nwhere\n") {
		t.Errorf("--fields after a transform:\n%s", out)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--aggregate", "count"}, "need --group-by or --pivot"},
		{[]string{"--pivot", "profile"}, "give rows:column"},
		{[]string{"--pivot", "profile:tags", "--group-by", "profile"}, "not both"},
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

// A schedule writes a saved selection to its folder on `schedule run`, and
// refuses what it cannot write.
func TestExportSchedules(t *testing.T) {
	p := editHome(t)
	run(t, "profile", "add", "work")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "--tag", "prd")
	sel := filepath.Join(t.TempDir(), "sel.json")
	if err := os.WriteFile(sel, []byte(`{"version":1,"grain":"host","columns":[{"field":"alias"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, "export", "selections", "save", "web", "--from", sel)
	out := t.TempDir()
	folder := filepath.Join(out, "reports")
	if got := run(t, "export", "schedule", "add", "web", "--to", folder); !strings.Contains(got, filepath.Join(folder, "web.csv")) {
		t.Errorf("add: %s", got)
	}
	run(t, "export", "schedule", "add", "web", "--format", "json", "--to", folder)
	if got := run(t, "export", "schedule", "list"); !strings.Contains(got, "web        csv") || !strings.Contains(got, "web.json") {
		t.Errorf("list:\n%s", got)
	}
	if got := run(t, "export", "schedule", "run"); strings.Count(got, "wrote 1 row(s)") != 2 {
		t.Errorf("run: %s", got)
	}
	b, err := os.ReadFile(filepath.Join(folder, "web.csv"))
	if err != nil || string(b) != "alias\nweb1\n" {
		t.Errorf("web.csv = %q, %v", b, err)
	}
	if fi, _ := os.Stat(filepath.Join(folder, "web.json")); runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("web.json is %v", fi.Mode())
	}
	if got := run(t, "export", "schedule", "run", "--quiet"); got != "" {
		t.Errorf("--quiet printed %q", got)
	}
	// A selection deleted under a schedule fails that schedule, loudly.
	run(t, "export", "selections", "delete", "web")
	if _, err := runErr(t, "export", "schedule", "run", "--quiet"); err == nil || !strings.Contains(err.Error(), "2 of 2 scheduled export(s) failed") {
		t.Errorf("run with the selection gone: %v", err)
	}
	if got := run(t, "export", "schedule", "remove", "web", "--format", "json"); !strings.Contains(got, "removed 1") {
		t.Errorf("remove one: %s", got)
	}
	run(t, "export", "schedule", "remove", "web")
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"export", "schedule", "add", "web", "--to", folder}, "no saved selection"},
		{[]string{"export", "schedule", "add", "x"}, "give --to"},
		{[]string{"export", "schedule", "add", "x", "--to", filepath.Join(p.SSHDir, "out")}, "inside"},
		{[]string{"export", "schedule", "remove", "web"}, `no schedule for "web"`},
	} {
		if _, err := runErr(t, c.args...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.args, err, c.want)
		}
	}
}
