package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Tabler 1.7 draws an accordion's chevron only inside .accordion-button-toggle,
// so a button without one looks like a plain heading and nobody can tell it
// opens. Every accordion button, in a template or built by a script, carries
// one.
func TestEveryAccordionShowsThatItOpens(t *testing.T) {
	button := regexp.MustCompile(`(?s)<button[^>]*class="accordion-button[^"]*"[^>]*>.*?</button>`)
	seen := 0
	err := fs.WalkDir(templateFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		b, err := fs.ReadFile(templateFS, p)
		if err != nil {
			return err
		}
		for _, m := range button.FindAllString(string(b), -1) {
			seen++
			if !strings.Contains(m, "accordion-button-toggle") {
				t.Errorf("%s: an accordion button has no .accordion-button-toggle: %.120s", p, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Scripts: a file that builds an accordion button builds its toggle too.
	err = fs.WalkDir(assetFS, "assets/js", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		b, err := fs.ReadFile(assetFS, p)
		if err != nil {
			return err
		}
		src := string(b)
		made := strings.Count(src, "`accordion-button") + strings.Count(src, `"accordion-button `) + strings.Count(src, `"accordion-button"`)
		toggles := strings.Count(src, `"accordion-button-toggle"`)
		seen += made
		if made > toggles {
			t.Errorf("%s builds %d accordion button(s) but only %d .accordion-button-toggle", p, made, toggles)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// about.html's licences and the export page's facets: if this drops, the
	// patterns above stopped matching and the test passes over nothing.
	if seen < 2 {
		t.Fatalf("found %d accordion buttons; the patterns no longer match the markup", seen)
	}
}

// The name in a list row opens that row in the slide-in panel, like the
// row menu's Open does; a modified click still opens a page.
func TestRowNamesOpenInThePanel(t *testing.T) {
	link := regexp.MustCompile(`<th scope="row"><a href="\{\{basePath\}\}/(hosts|keys)/[^"]*"([^>]*)>`)
	seen := 0
	for _, name := range []string{"hosts", "keys", "net", "validate"} {
		b, err := fs.ReadFile(templateFS, "templates/"+name+".html")
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range link.FindAllStringSubmatch(string(b), -1) {
			seen++
			if !strings.Contains(m[2], "data-inline") {
				t.Errorf("%s.html: row link %q does not open in the panel", name, m[0])
			}
		}
	}
	if seen < 4 {
		t.Fatalf("checked %d row links; want one per page at least", seen)
	}
}
