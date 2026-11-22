package structure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// BackupDir is where the files an approved plan moves are copied first:
// owner-only, under the state home's .state/moved/<UTC time>/.
func BackupDir(p paths.Paths, now time.Time) string {
	return filepath.Join(p.StateDir(), "moved", now.UTC().Format("20060102T150405Z"))
}

// StateFile is where the last check and the dismissals are kept.
func StateFile(p paths.Paths) string { return filepath.Join(p.StateDir(), "structure.json") }

// State is what the watcher remembers between checks.
type State struct {
	Checked     time.Time            `json:"checked,omitempty"`
	Suggestions []Suggestion         `json:"suggestions,omitempty"`
	Dismissed   map[string]time.Time `json:"dismissed,omitempty"` // id -> when
	Notified    map[string]time.Time `json:"notified,omitempty"`  // id -> when a notification named it
}

// LoadState reads the state; a missing file is an empty state.
func LoadState(p paths.Paths) (State, error) {
	var s State
	b, err := os.ReadFile(StateFile(p))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// SaveState writes the state owner-only.
func SaveState(p paths.Paths, s State) error {
	if err := os.MkdirAll(p.StateDir(), perms.DirMode); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := StateFile(p) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), perms.ConfigMode); err != nil {
		return err
	}
	return os.Rename(tmp, StateFile(p))
}

// Check is one watcher pass: the suggestions now, marked dismissed where the
// owner dismissed them, and which are new since the last notification. It
// saves the state (dropping dismissals and notes for suggestions that are
// gone) and moves nothing.
func Check(p paths.Paths, now time.Time) (all, fresh []Suggestion, err error) {
	all, err = Suggest(p)
	if err != nil {
		return nil, nil, err
	}
	st, err := LoadState(p)
	if err != nil {
		st = State{} // a damaged state file is rebuilt, not fatal
	}
	live := map[string]bool{}
	for i := range all {
		live[all[i].ID] = true
		if _, ok := st.Dismissed[all[i].ID]; ok {
			all[i].Dismissed = true
			continue
		}
		if _, ok := st.Notified[all[i].ID]; !ok && !all[i].Info {
			fresh = append(fresh, all[i])
		}
	}
	st.Dismissed = keep(st.Dismissed, live)
	st.Notified = keep(st.Notified, live)
	st.Checked, st.Suggestions = now.UTC(), all
	return all, fresh, SaveState(p, st)
}

// MarkNotified records that a notification named these suggestions, so the
// next check does not repeat them.
func MarkNotified(p paths.Paths, ss []Suggestion, now time.Time) error {
	st, err := LoadState(p)
	if err != nil {
		return err
	}
	if st.Notified == nil {
		st.Notified = map[string]time.Time{}
	}
	for _, s := range ss {
		st.Notified[s.ID] = now.UTC()
	}
	return SaveState(p, st)
}

// Dismiss hides suggestions from the list (and from notifications) until
// they change; undo restores them.
func Dismiss(p paths.Paths, ids []string, undo bool, now time.Time) error {
	st, err := LoadState(p)
	if err != nil {
		return err
	}
	if st.Dismissed == nil {
		st.Dismissed = map[string]time.Time{}
	}
	for _, id := range ids {
		if undo {
			delete(st.Dismissed, id)
		} else {
			st.Dismissed[id] = now.UTC()
		}
	}
	return SaveState(p, st)
}

// Find returns the suggestions with these ids (or id prefixes of at least 4
// characters), in the order asked; the ids no suggestion has come back too.
func Find(all []Suggestion, ids []string) (found []Suggestion, missing []string) {
	for _, id := range ids {
		var hit []Suggestion
		for _, s := range all {
			if s.ID == id || (len(id) >= 4 && len(id) < len(s.ID) && s.ID[:len(id)] == id) {
				hit = append(hit, s)
			}
		}
		if len(hit) != 1 {
			missing = append(missing, id)
			continue
		}
		found = append(found, hit[0])
	}
	return found, missing
}

func keep(m map[string]time.Time, live map[string]bool) map[string]time.Time {
	out := map[string]time.Time{}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if live[k] {
			out[k] = m[k]
		}
	}
	return out
}
