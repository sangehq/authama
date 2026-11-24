package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Every data table in a template can be copied from (CopyCells) and stacks
// into one card per row on a narrow screen (StackTable). A table added
// without them scrolls sideways on a laptop and offers no copy menu, which
// is how the Termius keys table read before both existed. The column picker
// in the CSV dialog is a form, not data, and is the one exception.
func TestEveryDataTableCopiesAndStacks(t *testing.T) {
	tag := regexp.MustCompile(`<table class="table[^"]*"[^>]*>`)
	seen := 0
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		body, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		for _, m := range tag.FindAllString(string(body), -1) {
			if strings.Contains(m, "csv-pick-table") {
				continue
			}
			seen++
			for _, attr := range []string{"data-copy-cells", "data-stack"} {
				if !strings.Contains(m, attr) {
					t.Errorf("%s: a table without %s: %s", path, attr, m)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 35 {
		t.Fatalf("read %d data tables, want at least 35: the pattern is not matching them", seen)
	}
}
