package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A saved selection survives a save and a load, and removing the last one
// writes the manifest as it was before any was saved.
func TestSelectionsRoundTripAndLeaveNoTrace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	m := &Manifest{Version: schemaVersion, Defaults: newDefaults(), Profiles: map[string]Profile{}}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	SetSelection(m, "prd-hosts", json.RawMessage(`{"version":1,"grain":"host","where":{"env":["prd"]}}`))
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := SelectionNames(back); len(got) != 1 || got[0] != "prd-hosts" {
		t.Fatalf("names = %v", got)
	}
	var sel map[string]any
	if err := json.Unmarshal(back.Defaults.Export.Selections["prd-hosts"], &sel); err != nil || sel["grain"] != "host" {
		t.Fatalf("selection = %v, %v", sel, err)
	}

	SetSelection(back, "prd-hosts", nil)
	if back.Defaults.Export != nil {
		t.Error("an empty export section was kept")
	}
	if err := back.Save(path); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("removing the last selection changed the file:\n%s\nwas\n%s", after, before)
	}
}

func TestABadSelectionIsRefused(t *testing.T) {
	for raw, want := range map[string]string{
		`{"Prd":{"version":1,"grain":"host"}}`:    "selection name",
		`{"../x":{"version":1,"grain":"host"}}`:   "selection name",
		`{"-x":{"version":1,"grain":"host"}}`:     "selection name",
		`{"ok":[1]}`:                              "not a JSON object",
		`{"ok":"text"}`:                           "not a JSON object",
		`{"ok":{"version":1,"grain":"host"}}`:     "",
		`{"a.b_c-9":{"version":1,"grain":"key"}}`: "",
	} {
		var sels map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &sels); err != nil {
			t.Fatal(err)
		}
		m := &Manifest{Version: schemaVersion, Defaults: newDefaults(), Profiles: map[string]Profile{}}
		m.Defaults.Export = &ExportDefaults{Selections: sels}
		err := m.Validate()
		if want == "" {
			if err != nil {
				t.Errorf("%s: %v", raw, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", raw, err, want)
		}
	}
}
