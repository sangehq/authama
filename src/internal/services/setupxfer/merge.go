package setupxfer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Policy settles a conflict: something the import carries that differs from
// what is here.
type Policy string

const (
	Keep    Policy = "keep"    // leave what is here
	Replace Policy = "replace" // take the import's
	Fail    Policy = "fail"    // refuse the whole import, writing nothing
)

// ParsePolicy reads --on-conflict.
func ParsePolicy(s string) (Policy, error) {
	switch Policy(s) {
	case Keep, Replace, Fail:
		return Policy(s), nil
	case "":
		return Keep, nil
	}
	return "", fmt.Errorf("--on-conflict is keep, replace or fail, not %q", s)
}

// Item states.
const (
	StateAdd     = "add"     // new here
	StateSame    = "same"    // already here, the same
	StateKept    = "kept"    // differs; what is here was kept
	StateReplace = "replace" // differs; the import's replaces it
)

// Item is one thing an import adds, leaves or replaces.
type Item struct {
	Area  string // manifest, profile, reserved_key, providers, tokens, setting, key, token
	Name  string
	State string
}

// Result is what a merge did, or would do.
type Result struct{ Items []Item }

// Counts tallies the items by state.
func (r Result) Counts() map[string]int {
	c := map[string]int{}
	for _, it := range r.Items {
		c[it.State]++
	}
	return c
}

// Conflicts lists the items that differ from what is here.
func (r Result) Conflicts() []Item {
	var out []Item
	for _, it := range r.Items {
		if it.State == StateKept || it.State == StateReplace {
			out = append(out, it)
		}
	}
	return out
}

// Format lists every item that is not already the same, then a tally.
func (r Result) Format() string {
	var b strings.Builder
	for _, it := range r.Items {
		if it.State == StateSame {
			continue
		}
		fmt.Fprintf(&b, "  %-8s %-12s %s\n", it.State, it.Area, it.Name)
	}
	c := r.Counts()
	fmt.Fprintf(&b, "%d to add, %d to replace, %d kept as they are here, %d already the same\n",
		c[StateAdd], c[StateReplace], c[StateKept], c[StateSame])
	return b.String()
}

// ConflictError is a Fail import that met conflicts; nothing was written.
type ConflictError struct{ Items []Item }

func (e ConflictError) Error() string {
	names := make([]string, 0, len(e.Items))
	for _, it := range e.Items {
		names = append(names, it.Area+" "+it.Name)
	}
	return fmt.Sprintf("the import differs from this home in %d place(s), and --on-conflict fail writes nothing: %s "+
		"(re-run with --on-conflict keep to keep what is here, or replace to take the import's)",
		len(e.Items), strings.Join(names, ", "))
}

// settle is the state of something present on both sides.
func settle(same bool, policy Policy) string {
	switch {
	case same:
		return StateSame
	case policy == Replace:
		return StateReplace
	}
	return StateKept // Keep, and Fail (which refuses before writing)
}

// Merge merges d into p's home under policy: the manifest profile by profile,
// the reserved keys one by one, the providers and the token config whole, and
// the settings one key at a time (never a machine-specific path). Every file
// is worked out before any is written, and a Fail import with a conflict
// writes nothing. Files are written owner-only.
func Merge(p paths.Paths, d Doc, policy Policy) (Result, error) {
	var res Result
	writes := map[string][]byte{}

	if len(d.Manifest) > 0 {
		items, body, err := mergeManifest(p.Manifest(), d.Manifest, policy)
		if err != nil {
			return res, err
		}
		res.Items = append(res.Items, items...)
		if body != nil {
			writes[p.Manifest()] = body
		}
	}
	for _, whole := range []struct {
		area, path string
		data       json.RawMessage
	}{{"providers", p.Providers(), d.Providers}, {"tokens", p.Tokens(), d.Tokens}} {
		if len(whole.data) == 0 {
			continue
		}
		it, body, err := mergeWhole(whole.area, whole.path, whole.data, policy)
		if err != nil {
			return res, err
		}
		res.Items = append(res.Items, it)
		if body != nil {
			writes[whole.path] = body
		}
	}
	if len(d.Settings) > 0 {
		items, body, err := mergeSettings(p.SettingsFile(), d.Settings, policy)
		if err != nil {
			return res, err
		}
		res.Items = append(res.Items, items...)
		if body != nil {
			writes[p.SettingsFile()] = body
		}
	}
	if policy == Fail {
		if c := res.Conflicts(); len(c) > 0 {
			return res, ConflictError{Items: c}
		}
	}
	targets := make([]string, 0, len(writes))
	for t := range writes {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		if err := fs.WriteTextAtomic(t, string(writes[t]), 0o600); err != nil {
			return res, err
		}
	}
	return res, nil
}

func sameJSON(a, b any) bool {
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(x, y)
}

func mergeManifest(path string, raw json.RawMessage, policy Policy) ([]Item, []byte, error) {
	in, err := parseManifest(raw)
	if err != nil {
		return nil, nil, err
	}
	var items []Item
	local, err := manifest.Load(path)
	if err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil, nil, err // a manifest that is here but broken is not replaced blind
		}
		local = nil
	}
	if local == nil || len(local.Profiles) == 0 {
		for _, name := range in.ProfileNames() {
			items = append(items, Item{Area: "profile", Name: name, State: StateAdd})
		}
		for _, name := range sortedNames(in.ReservedKeys) {
			items = append(items, Item{Area: "reserved_key", Name: name, State: StateAdd})
		}
		if local == nil || !sameJSON(local.Defaults, in.Defaults) {
			items = append(items, Item{Area: "manifest", Name: "defaults", State: StateAdd})
		}
		body, err := json.MarshalIndent(in, "", "  ")
		return items, append(body, '\n'), err
	}
	changed := false
	st := settle(sameJSON(local.Defaults, in.Defaults), policy)
	items = append(items, Item{Area: "manifest", Name: "defaults", State: st})
	if st == StateReplace {
		local.Defaults = in.Defaults
		changed = true
	}
	for _, name := range in.ProfileNames() {
		theirs := in.Profiles[name]
		ours, ok := local.Profiles[name]
		st := StateAdd
		if ok {
			st = settle(sameJSON(ours, theirs), policy)
		}
		items = append(items, Item{Area: "profile", Name: name, State: st})
		if st == StateAdd || st == StateReplace {
			local.SetProfile(name, theirs)
			changed = true
		}
	}
	for _, name := range sortedNames(in.ReservedKeys) {
		theirs := in.ReservedKeys[name]
		ours, ok := local.ReservedKeys[name]
		st := StateAdd
		if ok {
			st = settle(sameJSON(ours, theirs), policy)
		}
		items = append(items, Item{Area: "reserved_key", Name: name, State: st})
		if st == StateAdd || st == StateReplace {
			if local.ReservedKeys == nil {
				local.ReservedKeys = map[string]manifest.ReservedKey{}
			}
			local.ReservedKeys[name] = theirs
			changed = true
		}
	}
	if !changed {
		return items, nil, nil
	}
	if err := local.Validate(); err != nil {
		return nil, nil, fmt.Errorf("the merged manifest would not be valid (re-run with --on-conflict replace, or fix it here first): %w", err)
	}
	body, err := json.MarshalIndent(local, "", "  ")
	return items, append(body, '\n'), err
}

func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mergeWhole merges a file taken as one: added when absent, else settled.
func mergeWhole(area, path string, raw json.RawMessage, policy Policy) (Item, []byte, error) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return Item{}, nil, fmt.Errorf("the document's %s: %w", area, err)
	}
	pretty.WriteByte('\n')
	cur, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Item{Area: area, Name: path, State: StateAdd}, pretty.Bytes(), nil
	}
	if err != nil {
		return Item{}, nil, err
	}
	var a, b bytes.Buffer
	same := json.Compact(&a, cur) == nil && json.Compact(&b, raw) == nil && bytes.Equal(a.Bytes(), b.Bytes())
	st := settle(same, policy)
	if st == StateReplace {
		return Item{Area: area, Name: path, State: st}, pretty.Bytes(), nil
	}
	return Item{Area: area, Name: path, State: st}, nil, nil
}

// mergeSettings merges authama.json one setting at a time. The machine's own
// sections (paths) are neither read from the import nor touched here.
func mergeSettings(path string, raw json.RawMessage, policy Policy) ([]Item, []byte, error) {
	var theirs map[string]json.RawMessage
	if err := json.Unmarshal(raw, &theirs); err != nil {
		return nil, nil, fmt.Errorf("the document's settings: %w", err)
	}
	ours := map[string]json.RawMessage{}
	cur, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if len(cur) > 0 {
		if err := json.Unmarshal(cur, &ours); err != nil {
			return nil, nil, fmt.Errorf("%s does not parse, so settings are not merged into it: %w", path, err)
		}
	}
	var items []Item
	changed := false
	for _, section := range sortedNames(theirs) {
		if machineSections[section] {
			continue
		}
		val := theirs[section]
		var tsec map[string]json.RawMessage
		if json.Unmarshal(val, &tsec) != nil || tsec == nil {
			// $schema, version and the structured sections are scalars or
			// lists: taken whole.
			st := StateAdd
			if o, ok := ours[section]; ok {
				st = settle(sameRaw(o, val), policy)
			}
			if section == "$schema" || section == "version" {
				if st != StateAdd {
					continue
				}
			} else {
				items = append(items, Item{Area: "setting", Name: section, State: st})
			}
			if st == StateAdd || st == StateReplace {
				ours[section] = val
				changed = true
			}
			continue
		}
		osec := map[string]json.RawMessage{}
		if o, ok := ours[section]; ok {
			_ = json.Unmarshal(o, &osec)
		}
		secChanged := false
		for _, key := range sortedNames(tsec) {
			st := StateAdd
			if o, ok := osec[key]; ok {
				st = settle(sameRaw(o, tsec[key]), policy)
			}
			items = append(items, Item{Area: "setting", Name: section + "." + key, State: st})
			if st == StateAdd || st == StateReplace {
				osec[key] = tsec[key]
				secChanged = true
			}
		}
		if secChanged {
			b, err := json.Marshal(osec)
			if err != nil {
				return nil, nil, err
			}
			ours[section] = b
			changed = true
		}
	}
	if !changed {
		return items, nil, nil
	}
	body, err := json.MarshalIndent(ours, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	if _, err := settings.Parse(path, body); err != nil {
		return nil, nil, fmt.Errorf("the merged settings would not be valid: %w", err)
	}
	return items, append(body, '\n'), nil
}

func sameRaw(a, b json.RawMessage) bool {
	var x, y bytes.Buffer
	return json.Compact(&x, a) == nil && json.Compact(&y, b) == nil && bytes.Equal(x.Bytes(), y.Bytes())
}
