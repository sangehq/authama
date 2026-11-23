package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Browsers compile an input's pattern attribute with the regular
// expression v flag, where a character class may not hold an unescaped
// '-' outside a range, nor ( ) [ { } / | unescaped; a pattern that does not
// compile is ignored, so the field takes anything. Every pattern in the
// templates must compile that way.
func TestTemplatePatternsCompileWithTheVFlag(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("templates", "*.html"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no templates: %v", err)
	}
	attr := regexp.MustCompile(`\spattern="([^"]*)"`)
	class := regexp.MustCompile(`\[((?:\\.|[^\]\\])*)\]`)
	ranges := regexp.MustCompile(`\\.|[^\\-]-[^\\\]]`)
	seen := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range attr.FindAllStringSubmatch(string(b), -1) {
			seen++
			for _, c := range class.FindAllStringSubmatch(m[1], -1) {
				// Drop escapes and ranges; what is left must hold no syntax character.
				if rest := ranges.ReplaceAllString(c[1], ""); strings.ContainsAny(rest, "-()[{}/|") {
					t.Errorf("%s: pattern %q: the class [%s] needs its syntax characters escaped for the v flag", f, m[1], c[1])
				}
			}
		}
	}
	if seen < 5 {
		t.Errorf("found only %d patterns", seen)
	}
}
