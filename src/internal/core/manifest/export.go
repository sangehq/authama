package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
)

// ExportDefaults is what `authama export` keeps in the manifest, so it
// travels with it: selections saved for every machine that shares it.
type ExportDefaults struct {
	// Selections are saved selections by name, each the selection JSON the
	// export pipeline reads, with its own version inside. The manifest checks
	// the names and that each is a JSON object; the pipeline checks the rest
	// when it reads one.
	Selections map[string]json.RawMessage `json:"selections,omitempty"`
}

// selectionName is a saved selection's name: lower case, so it is the same
// name on a case-insensitive disk, and safe as a file name.
var selectionName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// CheckSelectionName says why name cannot name a saved selection, or nil.
func CheckSelectionName(name string) error {
	if !selectionName.MatchString(name) {
		return fmt.Errorf("selection name %q: use 1 to 63 of a-z, 0-9, '.', '_' and '-', starting with a letter or digit", name)
	}
	return nil
}

// SelectionNames are m's saved selections, sorted. A function, not a
// method: Manifest stays under its method ceiling.
func SelectionNames(m *Manifest) []string {
	if m.Defaults.Export == nil {
		return nil
	}
	out := make([]string, 0, len(m.Defaults.Export.Selections))
	for n := range m.Defaults.Export.Selections {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// SetSelection keeps raw as m's saved selection name, or removes it when raw
// is nil. An empty export section is dropped, so a manifest that never saved
// one is written as before.
func SetSelection(m *Manifest, name string, raw json.RawMessage) {
	if raw == nil {
		if m.Defaults.Export != nil {
			delete(m.Defaults.Export.Selections, name)
			if len(m.Defaults.Export.Selections) == 0 {
				m.Defaults.Export = nil
			}
		}
		return
	}
	if m.Defaults.Export == nil {
		m.Defaults.Export = &ExportDefaults{}
	}
	if m.Defaults.Export.Selections == nil {
		m.Defaults.Export.Selections = map[string]json.RawMessage{}
	}
	m.Defaults.Export.Selections[name] = raw
}

func checkExport(m *Manifest) error {
	if m.Defaults.Export == nil {
		return nil
	}
	for _, n := range SelectionNames(m) {
		if err := CheckSelectionName(n); err != nil {
			return fmt.Errorf("defaults.export.selections: %w", err)
		}
		raw := bytes.TrimSpace(m.Defaults.Export.Selections[n])
		var obj map[string]json.RawMessage
		if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &obj) != nil {
			return fmt.Errorf("defaults.export.selections.%s: not a JSON object", n)
		}
	}
	return nil
}
