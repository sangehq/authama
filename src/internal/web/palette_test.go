package web

import (
	"strings"
	"testing"
)

// Every shortcut is one key, then one key; no two entries share one; every
// page in the menu has one; and the sections hold every entry once.
func TestPaletteShortcutsAreUniqueAndCoverEveryPage(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		items := paletteItems(readOnly)
		if len(items) < 20 {
			t.Fatalf("readOnly=%v: only %d palette items inspected", readOnly, len(items))
		}
		seen := map[string]string{}
		for _, it := range items {
			if it.Keys == "" {
				if !strings.HasPrefix(it.Href, "/exports/") {
					t.Errorf("%s (%s) has no shortcut", it.ID, it.Href)
				}
				continue
			}
			if k := shortcutKeys(it.Keys); len(k) != 2 || len(k[0]) != 1 || len(k[1]) != 1 {
				t.Errorf("%s: shortcut %q is not two single keys", it.ID, it.Keys)
			}
			if other, dup := seen[it.Keys]; dup {
				t.Errorf("%s and %s share the shortcut %q", other, it.ID, it.Keys)
			}
			seen[it.Keys] = it.ID
		}
		n := 0
		for _, sec := range paletteSections(readOnly) {
			if len(sec.Items) == 0 {
				t.Errorf("section %s is empty", sec.Title)
			}
			n += len(sec.Items)
		}
		if n != len(items) {
			t.Errorf("readOnly=%v: sections hold %d entries, the palette %d", readOnly, n, len(items))
		}
	}
}

// The page carries the palette, grouped, with each entry's shortcut, and the
// shortcuts sheet built from the same list.
func TestThePaletteAndTheShortcutsSheetAreOnThePage(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/hosts").Body.String()
	for _, want := range []string{
		`data-command-palette`, `class="header-search"`, `aria-keyshortcuts="Control+K Meta+K /"`,
		`aria-label="Pages"`, `aria-label="Actions"`, `aria-label="Downloads"`,
		`id="pal-hosts"`, `data-palette-keys="g h"`, `data-palette-empty`,
		`id="keyboard-shortcuts"`, `data-shortcuts-toggle`, `data-shortcuts-open`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
}
