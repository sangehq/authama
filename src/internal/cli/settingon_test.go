package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/envnames"
)

// A boolean setting from authama.json reaches the environment as 1 or 0, and
// one set by hand may be true, yes or on: settingOn reads every spelling, and
// an unset one is its default.
func TestABooleanSettingIsReadInEverySpelling(t *testing.T) {
	for v, want := range map[string]bool{"1": true, "true": true, "on": true, "yes": true, "0": false, "false": false, "off": false} {
		t.Setenv(envnames.MCP, v)
		if got := settingOn(envnames.MCP, !want); got != want {
			t.Errorf("%q read as %v", v, got)
		}
	}
	t.Setenv(envnames.MCP, "")
	if !settingOn(envnames.MCP, true) || settingOn(envnames.MCP, false) {
		t.Error("an unset setting is not its default")
	}
}

// No code compares a setting with the word "true" or "false": a value from
// authama.json is 1 or 0, and such a comparison silently ignores it (as
// web.menu_export and web.auto_restart once did).
func TestNoSettingIsComparedWithTheWordTrueOrFalse(t *testing.T) {
	bad := regexp.MustCompile(`envnames\.Get\([^)]*\)\s*[!=]=\s*"(true|false)"`)
	files := 0
	for _, dir := range []string{".", "../web", "../services"} {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			files++
			b, _ := os.ReadFile(path)
			if loc := bad.Find(b); loc != nil {
				t.Errorf("%s compares a setting with a word: %s", path, loc)
			}
			return nil
		})
	}
	if files < 100 {
		t.Fatalf("read %d files; the walk is not finding the source", files)
	}
}
