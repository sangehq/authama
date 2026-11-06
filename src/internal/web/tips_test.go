package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var tipUse = regexp.MustCompile(`\b(?:tip|withTip) "([a-z0-9-]+)"`)

// Every explainer a template names is in the catalogue, and every catalogue
// entry is used: read from the templates' source, so a branch the page tests
// never render (dev mode, read-only) cannot hide a missing one.
func TestEveryTipATemplateNamesExists(t *testing.T) {
	used := map[string]bool{}
	files := 0
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		files++
		b, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		for _, m := range tipUse.FindAllStringSubmatch(string(b), -1) {
			used[m[1]] = true
			if _, ok := tips[m[1]]; !ok {
				t.Errorf("%s names tip %q, which web/tips.go does not have", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 25 || len(used) < 30 {
		t.Fatalf("read %d templates and found %d tips; the scan is not reading them", files, len(used))
	}
	for id := range tips {
		if !used[id] {
			t.Errorf("tip %q is in the catalogue and no template uses it", id)
		}
	}
}
