package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The rename to authama changed what people read, never what scripts parse.
// doctor --json keeps every 3.x key, in the 3.x order, and only gains
// "tokens" at the end; its values never carry a command spelling, so the
// `authama ssh` forms cannot leak into it. history --json stores the bare
// command ("profile add"), as 3.x did.

var doctorKeys3x = []string{"ok", "home", "ssh_dir", "providers_source", "preflight_ok", "agent",
	"known_hosts", "config_in_sync", "perm_issues", "old_keys", "stale_old_keys", "orphan_keys",
	"dangling_keys", "duplicate_keys", "unpinned_hosts", "alias_collisions", "stale_proxy",
	"shadowed_hosts", "raw_option_overlaps", "needed_tools", "clipboard", "stranded_legacy_home"}

func orderedKeys(t *testing.T, js string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(js))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("not a JSON object: %v\n%s", err, js)
	}
	var keys []string
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

func TestMachineReadableOutputKeepsTheV3Shape(t *testing.T) {
	b := tokenSandbox(t)
	run := func(args ...string) string {
		root := newRootCmd()
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs(append([]string{"--dev-root", b.root}, args...))
		_ = root.Execute()
		return out.String()
	}
	run("init")
	js := run("doctor", "--json")
	got := orderedKeys(t, js)
	want := append(append([]string{}, doctorKeys3x...), "tokens")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("doctor --json keys changed:\n got %v\nwant %v", got, want)
	}
	for _, spelling := range []string{"authama", "Authentication Access Manager"} {
		if strings.Contains(js, spelling) {
			t.Errorf("doctor --json carries %q:\n%s", spelling, js)
		}
	}
	run("ssh", "profile", "add", "work")
	hist := run("history", "--json")
	var first struct {
		Command string `json:"command"`
	}
	line := strings.SplitN(strings.TrimSpace(hist), "\n", 2)[0]
	if err := json.Unmarshal([]byte(line), &first); err != nil || first.Command != "profile add" {
		t.Errorf("history --json command = %q (%v), want the bare v3 form \"profile add\"\n%s", first.Command, err, hist)
	}
}
