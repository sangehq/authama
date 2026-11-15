package cli

import (
	"encoding/json"
	"testing"
)

// settings show --json prints JSON even when there is no authama.json: it
// printed a sentence, which no script can parse (review R-INC-05).
func TestSettingsShowJSONIsJSONWithNoFile(t *testing.T) {
	editHome(t)
	out := run(t, "settings", "show", "--json")
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("settings show --json printed something that is not JSON: %v\n%s", err, out)
	}
	if len(v) != 0 {
		t.Errorf("with no authama.json the settings are empty, got %v", v)
	}
}
