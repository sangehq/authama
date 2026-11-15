package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rawWriterExempt are the places allowed to write a file by truncating it in
// place (os.WriteFile, or O_TRUNC): each writes a new file, a backup, a
// temporary or staging copy, or a scratch file, never a live key, config,
// manifest, inventory or token file, which go through fs.WriteTextAtomic or an
// os.Root rename so an interrupted write cannot leave one half-written. The
// number is a ceiling: a new call site in a listed file fails too.
// Measured 2026-11-14 (review R-BUG-11) with the scan below: 19 sites.
var rawWriterExempt = map[string]struct {
	n   int
	why string
}{
	"internal/tabler/tabler.go":                   {1, "vendors Tabler assets into the source tree (a build tool, not a home file)"},
	"internal/cli/serve.go":                       {1, "--token-file: a file the caller names, written once at start, 0600"},
	"internal/cli/man.go":                         {1, "writes generated man pages into a directory the caller names"},
	"internal/util/scheduler/scheduler_linux.go":  {2, "systemd user units, regenerated whole on every install"},
	"internal/util/scheduler/scheduler_darwin.go": {1, "a launchd plist, regenerated whole on every install"},
	"internal/services/migratesvc/migratesvc.go":  {2, "the migrated marker, and copies into a new home being built"},
	"internal/services/change/change.go":          {1, "the preview's shadow home, a temporary copy"},
	"internal/services/importer/backup.go":        {1, "a timestamped backup copy"},
	"internal/services/initsvc/initsvc.go":        {1, "a timestamped backup copy taken before init --force"},
	"internal/services/snapshots/snapshots.go":    {1, "copies a chosen snapshot to a safe name before restoring it"},
	"internal/services/tokensvc/aliases.go":       {1, "a backup copy beside a token file, before it is changed"},
	"internal/services/bundler/bundler.go":        {3, "a new bundle's .sha256 and .contents sidecars, and the temp file of an os.Root atomic write"},
	"internal/services/rotator/summary.go":        {1, "the list of failed keys, a state-home record rewritten whole"},
	"internal/services/vmsync/probe.go":           {1, "a known_hosts in a temporary directory, for one probe"},
	"internal/services/vmsync/apply.go":           {1, "a timestamped copy of a file it is about to change"},
}

var rawWriter = regexp.MustCompile(`os\.WriteFile\(|O_TRUNC`)

// Live files are replaced by rename, never truncated in place: an undo that
// rewrote the manifest with os.WriteFile could leave it half-written if
// interrupted, against the rule manifest.go states (review R-BUG-11).
func TestNoLiveFileIsTruncatedInPlace(t *testing.T) {
	root := repoRoot(t)
	seen := map[string]int{}
	scanned := 0
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/util/fs/") {
			return nil // the atomic writers themselves
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		if n := len(rawWriter.FindAll(b, -1)); n > 0 {
			seen[rel] = n
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 300 {
		t.Fatalf("scanned %d Go files under internal/; the walk is not reaching the tree", scanned)
	}
	for rel, n := range seen {
		ex, ok := rawWriterExempt[rel]
		switch {
		case !ok:
			t.Errorf("%s truncates a file in place %d time(s): replace a live file with fs.WriteTextAtomic, or add an exemption with its reason", rel, n)
		case n > ex.n:
			t.Errorf("%s truncates in place %d time(s), its exemption allows %d (%s)", rel, n, ex.n, ex.why)
		}
	}
	for rel, ex := range rawWriterExempt {
		if seen[rel] < ex.n {
			t.Errorf("exemption for %s allows %d but finds %d: lower it (%s)", rel, ex.n, seen[rel], ex.why)
		}
	}
}
