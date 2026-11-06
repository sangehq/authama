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

// controlTag is the opening tag of a button, a link styled as a button or a
// menu entry, or a menu link: the things a person clicks to make something
// happen. Template actions inside the tag ({{…}}) are kept whole.
var controlTag = regexp.MustCompile(`<(?:button|a)\b(?:[^>{]|\{\{.*?\}\})*>`)

// templateComment is a {{/* … */}} comment, whose examples are not controls.
var templateComment = regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)

// Every control on every page says what using it does: a catalogue tip, a
// header action's tip, or Bootstrap's own tooltip or popover with a title.
// The only controls exempt are the ones whose meaning is their shape: a
// dialog's close button, Cancel (both data-bs-dismiss), and an accordion's
// header, which shows the text it is named for.
func TestEveryControlHasAnExplainer(t *testing.T) {
	controls, files := 0, 0
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		files++
		b, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		src := templateComment.ReplaceAllString(string(b), "")
		for _, tag := range controlTag.FindAllString(src, -1) {
			if strings.HasPrefix(tag, "<a ") && !regexp.MustCompile(`class="[^"]*\b(btn|dropdown-item|nav-link)\b`).MatchString(tag) {
				continue // a link in running text says where it goes
			}
			if strings.Contains(tag, "data-bs-dismiss") || strings.Contains(tag, "btn-close") || strings.Contains(tag, "accordion-button") {
				continue
			}
			controls++
			explained := strings.Contains(tag, "{{tip ") || strings.Contains(tag, "data-tip=") ||
				strings.Contains(tag, `template "action_attrs"`) || strings.Contains(tag, ` title=`)
			if !explained {
				t.Errorf("%s: a control without an explainer: %s", path, shorten(tag, 160))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 25 || controls < 120 {
		t.Fatalf("read %d templates and %d controls; the scan is not reading them", files, controls)
	}
}
