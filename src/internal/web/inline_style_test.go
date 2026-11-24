package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// The Content-Security-Policy allows styles from 'self' only, so a style=""
// attribute in a template is dropped by the browser without a word. Styles
// go in assets/app.css as a class.
func TestNoTemplateUsesAnInlineStyle(t *testing.T) {
	inline := regexp.MustCompile(`\sstyle="`)
	read := 0
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		read++
		body, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(body), "\n") {
			if inline.MatchString(line) {
				t.Errorf("%s:%d: an inline style, which the CSP blocks: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ContentSecurityPolicy, "style-src 'self';") {
		t.Fatal("the CSP's style-src changed; revisit this guard")
	}
	if read < 60 {
		t.Fatalf("read %d templates, want at least 60: the walk is not finding them", read)
	}
}
