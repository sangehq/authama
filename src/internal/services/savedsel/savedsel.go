// Package savedsel keeps export selections by name, in two stores: this
// account's own (a file each under the state home, private to it) and the
// manifest's (defaults.export.selections, so they travel with it). Both are
// read; on a name in both, the account's own wins and the manifest's is
// listed as shadowed. The export.save_to setting says where a save goes.
package savedsel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/util/envnames"
	fsutil "github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// The stores.
const (
	StoreUser     = "user"
	StoreManifest = "manifest"
)

// Stores lists them, the default first.
var Stores = []string{StoreUser, StoreManifest}

// Reserved are names a saved selection cannot take: the built-in lists, and
// the words `authama export` gives its own meaning.
var Reserved = []string{"selections", "schedule"}

// Saved is one saved selection.
type Saved struct {
	Name      string              `json:"name"`
	Store     string              `json:"store"`
	Selection selection.Selection `json:"selection"`
	// Shadowed marks a manifest selection hidden by one of this account's
	// own with the same name.
	Shadowed bool `json:"shadowed,omitempty"`
	// Err says why the stored selection cannot be read (another version,
	// say). It is listed, so it can be seen and deleted, never used.
	Err string `json:"error,omitempty"`
}

// ErrNotFound is a name no store has.
var ErrNotFound = errors.New("no saved selection")

// DefaultStore is where a save goes when none is named: export.save_to, or
// the user's own store.
func DefaultStore(get envnames.Getenv) string {
	if v := envnames.Get(get, envnames.ExportSaveTo); slices.Contains(Stores, v) {
		return v
	}
	return StoreUser
}

// CheckName says why name cannot name a saved selection, or nil.
func CheckName(name string) error {
	if err := manifest.CheckSelectionName(name); err != nil {
		return err
	}
	if _, ok := selection.Presets[name]; ok || slices.Contains(Reserved, name) {
		return fmt.Errorf("%q is a built-in list's name; choose another", name)
	}
	return nil
}

// CheckStore says why store is not a store, or nil.
func CheckStore(store string) error {
	if !slices.Contains(Stores, store) {
		return fmt.Errorf("no store %q: use %s", store, strings.Join(Stores, " or "))
	}
	return nil
}

func userFile(p paths.Paths, name string) string {
	return filepath.Join(paths.ExportSelectionsDir(p), name+".json")
}

// List is every saved selection in both stores, by name, the user's first
// on a name in both. m may be nil when the manifest cannot be read.
func List(p paths.Paths, m *manifest.Manifest) ([]Saved, error) {
	var out []Saved
	entries, err := os.ReadDir(paths.ExportSelectionsDir(p))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	mine := map[string]bool{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() || CheckName(name) != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(paths.ExportSelectionsDir(p), e.Name()))
		out = append(out, read(name, StoreUser, b, err))
		mine[name] = true
	}
	if m != nil {
		for _, name := range manifest.SelectionNames(m) {
			s := read(name, StoreManifest, m.Defaults.Export.Selections[name], nil)
			s.Shadowed = mine[name]
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Store == StoreUser
	})
	return out, nil
}

func read(name, store string, b []byte, err error) Saved {
	s := Saved{Name: name, Store: store}
	if err == nil {
		s.Selection, err = selection.Parse(b)
	}
	if err != nil {
		s.Err = err.Error()
	}
	return s
}

// Get is the saved selection name: the user's own when there is one, else
// the manifest's. One that cannot be read is an error saying why.
func Get(p paths.Paths, m *manifest.Manifest, name string) (Saved, error) {
	all, err := List(p, m)
	if err != nil {
		return Saved{}, err
	}
	for _, s := range all {
		if s.Name == name && !s.Shadowed {
			if s.Err != "" {
				return s, fmt.Errorf("saved selection %s (%s): %s", name, s.Store, s.Err)
			}
			return s, nil
		}
	}
	return Saved{}, fmt.Errorf("%w named %q", ErrNotFound, name)
}

// Encode is sel as a store keeps it: indented JSON, its version set.
func Encode(sel selection.Selection) ([]byte, error) {
	sel.Version = selection.Version
	return json.MarshalIndent(sel, "", "  ")
}

// Save keeps sel as name in store. A manifest save goes through the editor,
// so it is validated and audited like any manifest change; the caller takes
// the lock and the snapshot a mutation takes.
func Save(p paths.Paths, store, name string, sel selection.Selection) error {
	if err := CheckName(name); err != nil {
		return err
	}
	if err := CheckStore(store); err != nil {
		return err
	}
	if !slices.Contains(selection.Grains, sel.Grain) {
		return fmt.Errorf("selection: no grain %q, one of %s", sel.Grain, strings.Join(selection.Grains, ", "))
	}
	b, err := Encode(sel)
	if err != nil {
		return err
	}
	if store == StoreManifest {
		return editor.SetSelection(editor.New(p), name, b)
	}
	if err := os.MkdirAll(paths.ExportSelectionsDir(p), 0o700); err != nil {
		return err
	}
	return fsutil.WriteTextAtomic(userFile(p, name), string(b)+"\n", 0o600)
}

// Delete removes name from store.
func Delete(p paths.Paths, store, name string) error {
	if err := CheckStore(store); err != nil {
		return err
	}
	if store == StoreManifest {
		return editor.SetSelection(editor.New(p), name, nil)
	}
	if err := manifest.CheckSelectionName(name); err != nil {
		return err
	}
	err := os.Remove(userFile(p, name))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w named %q in the user store", ErrNotFound, name)
	}
	return err
}
