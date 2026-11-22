package manifest

import (
	"path/filepath"
	"strings"
	"testing"
)

// A manifest that skipped validation (built in memory, or a document being
// previewed) still cannot point a key path out of the key root.
func TestAnUnvalidatedNameStaysInsideTheKeyRoot(t *testing.T) {
	ssh := filepath.Join(t.TempDir(), ".ssh")
	m := &Manifest{Profiles: map[string]Profile{
		"../../etc": {},
		"work":      {Category: "../x"},
	}}
	m.Defaults.KeyRoot = "../outside"
	inside := func(p string) bool { return strings.HasPrefix(p, ssh+string(filepath.Separator)) }
	for _, p := range []string{
		m.ProfileDir(ssh, "../../etc"),
		m.ProfileDir(ssh, "work"),
		m.KeyPath(ssh, KeyRef{Profile: "work", KeyName: "../../id"}),
		filepath.Join(ssh, filepath.FromSlash(CategoryConfigRelIn("../r", "../c"))),
	} {
		if !inside(p) || strings.Contains(filepath.ToSlash(p), "..") {
			t.Errorf("%s is not inside %s", p, ssh)
		}
	}
	if got := m.ProfileRel("good"); got != "_invalid-name_/good" {
		t.Errorf("an invalid key root: %s", got)
	}
}
