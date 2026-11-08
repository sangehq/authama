package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var tipUse = regexp.MustCompile(`\b(?:tip|desc|withTip) "([a-z0-9-]+)"`)

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
		for _, loc := range controlTag.FindAllStringIndex(src, -1) {
			tag := src[loc[0]:loc[1]]
			// A disabled control gets no hover, so its explainer sits on a
			// wrapping element (Bootstrap's pattern for disabled tooltips).
			if before := src[:loc[0]]; strings.Contains(tag, " disabled") {
				if i := strings.LastIndex(before, "<"); i >= 0 && (strings.Contains(before[i:], "{{tip ") || strings.Contains(before[i:], "{{desc ")) {
					controls++
					continue
				}
			}
			if strings.HasPrefix(tag, "<a ") && !regexp.MustCompile(`class="[^"]*\b(btn|dropdown-item|nav-link)\b`).MatchString(tag) {
				continue // a link in running text says where it goes
			}
			if strings.Contains(tag, "data-bs-dismiss") || strings.Contains(tag, "btn-close") || strings.Contains(tag, "accordion-button") {
				continue
			}
			controls++
			explained := strings.Contains(tag, "{{tip ") || strings.Contains(tag, "data-tip=") ||
				strings.Contains(tag, "{{desc ") || strings.Contains(tag, "aria-description=") ||
				strings.Contains(tag, "data-bs-title=") || // a help popover's heading
				strings.Contains(tag, "nav-menu-item") || // a menu entry's description is the line under it
				strings.Contains(tag, `template "action_attrs"`)
			if !explained {
				t.Errorf("%s: a control without an explainer: %s", path, shorten(tag, 160))
			}
			if strings.Contains(tag, ` title=`) && (strings.Contains(tag, "{{tip ") || strings.Contains(tag, "data-tip=")) {
				t.Errorf("%s: a control with both a tip and a title shows two tooltips: %s", path, shorten(tag, 160))
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

// tooltipOpen is an opening tag that would give its element a tooltip: a tip,
// a Bootstrap tooltip, or a native title (which the browser shows as one).
var tooltipOpen = regexp.MustCompile(`<([a-z][a-z0-9]*)\b((?:[^>{]|\{\{.*?\}\})*(?:\{\{tip |data-tip=|data-bs-toggle="tooltip"| title=)(?:[^>{]|\{\{.*?\}\})*)>`)

// shownText strips what an element's content shows no text with: icons,
// screen-reader-only spans, tags and template actions that print nothing.
func shownText(inner string) string {
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`\{\{icon [^}]*\}\}`),
		regexp.MustCompile(`(?s)<span class="visually-hidden">.*?</span>`),
		regexp.MustCompile(`(?s)<kbd[^>]*>.*?</kbd>`),
		regexp.MustCompile(`<[^>]+>`),
		regexp.MustCompile(`\{\{-?\s*(end|else|if [^}]*|range [^}]*|with [^}]*)\s*-?\}\}`),
	} {
		inner = re.ReplaceAllString(inner, "")
	}
	return strings.TrimSpace(inner)
}

// Tooltips are for icon-only controls. Nothing that shows its own text, no
// dropdown toggle and no menu entry carries a tooltip or a native title: it
// would cover what is pointed at and repeat its label. Their explainer is
// aria-description, or a line of text in the menu.
func TestTooltipsOnlyOnIconOnlyControls(t *testing.T) {
	seen := 0
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		b, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		src := templateComment.ReplaceAllString(string(b), "")
		for _, m := range tooltipOpen.FindAllStringSubmatchIndex(src, -1) {
			tag, name := src[m[0]:m[1]], src[m[2]:m[3]]
			if strings.Contains(tag, `data-bs-toggle="popover"`) || name == "title" {
				continue // a help popover opens on focus or click; <title> is the page's
			}
			seen++
			switch {
			case strings.Contains(tag, `data-bs-toggle="dropdown"`):
				t.Errorf("%s: a dropdown toggle with a tooltip or title: %s", path, shorten(tag, 160))
				continue
			case strings.Contains(tag, "dropdown-item"), strings.Contains(tag, "nav-link") && !strings.Contains(tag, "px-0"):
				t.Errorf("%s: a menu entry or nav link with a tooltip or title: %s", path, shorten(tag, 160))
				continue
			case strings.Contains(tag, " title=") && (strings.Contains(tag, "{{tip ") || strings.Contains(tag, "data-tip=") || strings.Contains(tag, "tooltip")):
				t.Errorf("%s: a native title beside a tooltip shows two: %s", path, shorten(tag, 160))
			}
			if name == "input" || name == "select" || name == "textarea" {
				t.Errorf("%s: a form field with a tooltip or title (it has a label): %s", path, shorten(tag, 160))
				continue
			}
			end := strings.Index(src[m[1]:], "</"+name+">")
			if end < 0 {
				continue
			}
			if text := shownText(src[m[1] : m[1]+end]); text != "" {
				t.Errorf("%s: %q shows its own text and also has a tooltip or title: %s", path, shorten(text, 40), shorten(tag, 120))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The header's icon buttons, the panel's back and full-page buttons, the
	// theme switch and the copy button carry tooltips; if the count drops to
	// nothing, the pattern stopped matching.
	if seen < 6 {
		t.Fatalf("checked %d elements with a tooltip or title; the pattern no longer matches", seen)
	}
}
