package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// keyRootLiteral matches a "profiles" string literal, or one starting with
// "profiles/": a hand-built path into the key tree.
var keyRootLiteral = regexp.MustCompile(`"profiles(/[^"]*)?"`)

// keyRootExempt are the files allowed to spell the default key root, each
// with a reason and the number of lines measured when the key root became
// configurable (2026-11-05). A count above its ceiling fails, so an entry
// cannot quietly cover a new path; one below it fails too, so a stale entry
// is removed rather than left to cover the next.
var keyRootExempt = map[string]struct {
	lines  int
	reason string
}{
	"internal/core/manifest/keyroot.go":          {1, "DefaultKeyRoot itself: the one place the default is spelled"},
	"internal/core/manifest/manifest.go":         {2, `the manifest's own JSON key "profiles", not a path`},
	"internal/util/perms/perms.go":               {1, "deprecated IterManagedPaths delegating to the default; util sits below core and cannot import manifest"},
	"internal/cli/config.go":                     {1, "legacy per-profile known_hosts migration: those files only ever existed under profiles/, before key_root"},
	"internal/services/knownhosts/knownhosts.go": {1, "legacy per-profile known_hosts migration, as above"},
	"internal/services/change/actions.go":        {1, "legacy per-profile known_hosts migration, as above"},
	"internal/web/render.go":                     {1, "the Profiles page's nav key and URL, not a path under ~/.ssh"},
	"internal/web/routes.go":                     {1, "the Profiles page's template and view name"},
	"internal/services/overview/overview.go":     {1, `the English plural "profiles"`},
	"internal/services/selection/presets.go":     {1, `the built-in export preset named for the profiles list (?preset=profiles), not a path (measured 2026-11-06)`},
}

// Every path into the key tree is built from manifest.KeyRoot, so a
// configured defaults.key_root is honoured everywhere. A "profiles" literal
// in code outside the exemptions is a path that would silently ignore it.
func TestNoPathHardCodesTheKeyRoot(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	files := 0
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			files++
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			for n, line := range strings.Split(string(b), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") || strings.Contains(line, `json:"profiles"`) {
					continue
				}
				if keyRootLiteral.MatchString(line) {
					counts[rel]++
					if _, ok := keyRootExempt[rel]; !ok {
						t.Errorf("%s:%d spells the key root by hand; build the path from manifest.KeyRoot:\n\t%s", rel, n+1, trimmed)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 250 { // 265 measured 2026-11-05
		t.Fatalf("inspected %d Go files; the walk has stopped matching the tree", files)
	}
	for file, ex := range keyRootExempt {
		if counts[file] != ex.lines {
			t.Errorf("%s spells the key root on %d lines, its exemption allows exactly %d (%s): update the measurement or the code",
				file, counts[file], ex.lines, ex.reason)
		}
	}
}
