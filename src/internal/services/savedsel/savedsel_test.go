package savedsel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/selection"
	"github.com/simtabi/authama/src/internal/util/paths"
)

func home(t *testing.T) paths.Paths {
	t.Helper()
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg"), StateRoot: filepath.Join(base, "state")}
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1,"defaults":{},"profiles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, p paths.Paths) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

var prd = selection.Selection{Grain: selection.GrainHost, Where: selection.Where{Env: []string{"prd"}}}

// Both stores are read; on a name in both, the user's own wins and the
// manifest's is listed as shadowed.
func TestBothStoresAndTheUsersWins(t *testing.T) {
	p := home(t)
	if err := Save(p, StoreManifest, "prd", prd); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, StoreManifest, "keys-only", selection.Selection{Grain: selection.GrainKey}); err != nil {
		t.Fatal(err)
	}
	mine := prd
	mine.Where.Env = []string{"stg"}
	if err := Save(p, StoreUser, "prd", mine); err != nil {
		t.Fatal(err)
	}
	m := load(t, p)
	all, err := List(p, m)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range all {
		got = append(got, s.Name+"/"+s.Store+map[bool]string{true: "/shadowed"}[s.Shadowed])
	}
	if want := "keys-only/manifest prd/user prd/manifest/shadowed"; strings.Join(got, " ") != want {
		t.Errorf("list = %q, want %q", got, want)
	}
	s, err := Get(p, m, "prd")
	if err != nil || s.Store != StoreUser || s.Selection.Where.Env[0] != "stg" || s.Selection.Version != selection.Version {
		t.Errorf("get = %+v, %v", s, err)
	}
	// The user file is private, and its name is the selection's.
	fi, err := os.Stat(filepath.Join(paths.ExportSelectionsDir(p), "prd.json"))
	if err != nil || (fi.Mode().Perm()&0o077 != 0 && os.PathSeparator == '/') {
		t.Errorf("user file: %v %v", fi, err)
	}
	// Deleting the user's brings the manifest's back.
	if err := Delete(p, StoreUser, "prd"); err != nil {
		t.Fatal(err)
	}
	if s, err := Get(p, load(t, p), "prd"); err != nil || s.Store != StoreManifest || s.Selection.Where.Env[0] != "prd" {
		t.Errorf("after delete: %+v, %v", s, err)
	}
	if err := Delete(p, StoreManifest, "prd"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(p, load(t, p), "prd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after both deletes: %v", err)
	}
}

func TestBadNamesStoresAndSelectionsAreRefused(t *testing.T) {
	p := home(t)
	for _, c := range []struct {
		store, name string
		sel         selection.Selection
		want        string
	}{
		{StoreUser, "Prd", prd, "selection name"},
		{StoreUser, "../x", prd, "selection name"},
		{StoreUser, "hosts", prd, "built-in list"},
		{StoreManifest, "selections", prd, "built-in list"},
		{"cloud", "x", prd, `no store "cloud"`},
		{StoreUser, "x", selection.Selection{Grain: "planet"}, `no grain "planet"`},
	} {
		if err := Save(p, c.store, c.name, c.sel); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s/%s: %v, want %q", c.store, c.name, err, c.want)
		}
	}
	for store, want := range map[string]string{StoreUser: "no saved selection", StoreManifest: "the manifest has no saved selection"} {
		if err := Delete(p, store, "nope"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("delete from %s: %v", store, err)
		}
	}
}

// A stored selection that cannot be read is listed with why, never used and
// never dropped, so it can be seen and deleted.
func TestAnUnreadableSelectionIsListedNotUsed(t *testing.T) {
	p := home(t)
	if err := os.MkdirAll(paths.ExportSelectionsDir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.ExportSelectionsDir(p), "future.json"), []byte(`{"version":2,"grain":"host"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Not a saved selection at all: a stray file, a bad name.
	_ = os.WriteFile(filepath.Join(paths.ExportSelectionsDir(p), "notes.txt"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(paths.ExportSelectionsDir(p), "Bad.json"), []byte("{}"), 0o600)
	all, err := List(p, load(t, p))
	if err != nil || len(all) != 1 || all[0].Name != "future" || !strings.Contains(all[0].Err, "version 2") {
		t.Fatalf("list = %+v, %v", all, err)
	}
	if _, err := Get(p, load(t, p), "future"); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("get: %v", err)
	}
}

func TestDefaultStoreReadsTheSetting(t *testing.T) {
	for v, want := range map[string]string{"": StoreUser, "manifest": StoreManifest, "user": StoreUser, "cloud": StoreUser} {
		if got := DefaultStore(func(k string) string {
			if k == "AUTHAMA_EXPORT_SAVE_TO" {
				return v
			}
			return ""
		}); got != want {
			t.Errorf("%q: %s, want %s", v, got, want)
		}
	}
}
