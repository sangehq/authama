package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

// A raw option read as a number or a boolean is written back the same way
// while its value is unchanged: saving the manifest for an unrelated edit
// used to turn ServerAliveInterval 60 into "60" and true into "True".
func TestUnchangedOptionsKeepTheirJSONType(t *testing.T) {
	var o OrderedOptions
	if err := json.Unmarshal([]byte(`{"ServerAliveInterval": 60, "Compression": true, "User": "git"}`), &o); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != `{"ServerAliveInterval":60,"Compression":true,"User":"git"}` {
		t.Fatalf("round trip: %s", got)
	}
	// A value set afterwards is written as the string it now is.
	o.vals["ServerAliveInterval"] = "30"
	out, _ = json.Marshal(o)
	if !strings.Contains(string(out), `"ServerAliveInterval":"30"`) {
		t.Fatalf("changed value: %s", out)
	}
}
