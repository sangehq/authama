package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// oldNameLiteral matches a string literal naming the tool as it was called
// before authama 0.1.1: ssh-manager, sshmgr, or the ssh_manager scheduler
// label.
var oldNameLiteral = regexp.MustCompile("\"[^\"\\n]*(?:sshmgr|ssh-manager|ssh_manager)[^\"\\n]*\"|`[^`\\n]*(?:sshmgr|ssh-manager|ssh_manager)[^`\\n]*`")

// oldNameReaders are the files allowed to spell an old name, each with what it
// reads and the number of lines measured on 2026-11-07, when every writer of
// an old name was gone. Each of these READS something an older release left
// on disk or on a provider account; none writes an old name. A count above its
// ceiling fails, so an entry cannot quietly cover new code that writes one; a
// count below it fails too, so a reader that goes is taken off the list.
var oldNameReaders = map[string]struct {
	lines  int
	reason string
}{
	"internal/core/renderer/renderer.go":   {4, "the managed-block markers older releases wrote into ~/.ssh/config"},
	"internal/core/providers/cloud.go":     {1, "the title prefixes of keys older releases uploaded"},
	"internal/services/knownhosts/hash.go": {1, "the tag on known_hosts pins older releases wrote"},
	"internal/services/bundler/backup.go":  {1, "the file name prefix of bundles older releases wrote"},
	"internal/web/jobs.go":                 {1, "the same bundle name, for the web UI's restore"},
	"internal/util/scheduler/scheduler.go": {1, "the label older releases installed the notifier under, to remove it"},
	"internal/util/paths/paths.go":         {5, "the config homes older releases used, for migration"},
	"internal/cli/migrate.go":              {1, "migrate's help, naming the homes it copies from"},
	"internal/cli/clean.go":                {1, "clean's confirmation, naming the pins it prunes"},
}

// Nothing authama writes may carry the names it shipped under before 0.1.1.
// The only places that spell them are the readers above, which recognise what
// older releases left behind; comments are not counted.
func TestOnlyTheOldDataReadersSpellTheOldNames(t *testing.T) {
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
			b, err := os.ReadFile(path) //nolint:gosec // a source file under the module
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			for n, line := range strings.Split(string(b), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") || !oldNameLiteral.MatchString(line) {
					continue
				}
				counts[rel]++
				if _, ok := oldNameReaders[rel]; !ok {
					t.Errorf("%s:%d spells a name the tool had before authama; nothing may write one:\n\t%s", rel, n+1, trimmed)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 290 { // 310 measured 2026-11-07
		t.Fatalf("inspected %d Go files; the walk has stopped matching the tree", files)
	}
	for file, r := range oldNameReaders {
		if counts[file] != r.lines {
			t.Errorf("%s spells an old name on %d lines, its entry allows exactly %d (%s): update the measurement or the code",
				file, counts[file], r.lines, r.reason)
		}
	}
}
