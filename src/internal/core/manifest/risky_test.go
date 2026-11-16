package manifest

import (
	"strings"
	"testing"
)

// The raw SSH options a forged bundle could plant to redirect or weaken
// connections are named, with where they are; the ordinary ones, and the
// same keywords pointed somewhere safe, are not (audit A-SEC-13).
func TestRiskyOptionsAreNamed(t *testing.T) {
	doc := `{
	  "defaults": {"global_options": {
	    "AddKeysToAgent": "yes", "ForwardAgent": "yes", "StrictHostKeyChecking": "no",
	    "ControlPath": "/tmp/shared-%C", "IdentityAgent": "/tmp/other.sock"
	  }},
	  "profiles": {"work": {"hosts": [
	    {"alias": "a", "raw_options": {"ForwardAgent": "yes", "StrictHostKeyChecking": "no",
	      "ControlPath": "~/.ssh/cm/%C", "IdentityAgent": "SSH_AUTH_SOCK"}},
	    {"alias": "b", "raw_options": {"controlpath": "/run/x/../../tmp/%C", "IdentityAgent": "~/.ssh/../agent.sock"}},
	    {"alias": "c", "raw_options": {"IdentityAgent": "none", "ControlPath": "$XDG_RUNTIME_DIR/ssh-%C"}}
	  ]}}
	}`
	got, err := RiskyOptionsInJSON([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, r := range got {
		names = append(names, r.Where+" "+r.Key)
		if r.Why == "" || !strings.Contains(r.String(), r.Value) {
			t.Errorf("%+v does not say what it is and why", r)
		}
	}
	want := []string{
		"defaults.global_options ForwardAgent",
		"defaults.global_options StrictHostKeyChecking",
		"defaults.global_options ControlPath",
		"defaults.global_options IdentityAgent",
		"work/b.raw_options controlpath",
		"work/b.raw_options IdentityAgent",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Errorf("risky options:\n%s\nwant:\n%s", strings.Join(names, "\n"), strings.Join(want, "\n"))
	}
	if got, err := RiskyOptionsInJSON([]byte(`{"v":1}`)); err != nil || len(got) != 0 {
		t.Errorf("a document with no options: %v %v", got, err)
	}
	if _, err := RiskyOptionsInJSON([]byte(`{`)); err == nil {
		t.Error("a document that does not parse was accepted")
	}
}
