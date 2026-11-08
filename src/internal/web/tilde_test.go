package web

import (
	"path/filepath"
	"testing"
)

func TestTildeShortensOnlyTheHomeDirectory(t *testing.T) {
	homes := []string{filepath.FromSlash("/private/var/h/ann"), filepath.FromSlash("/var/h/ann")}
	for in, want := range map[string]string{
		"/private/var/h/ann":         "~",
		"/private/var/h/ann/.ssh":    "~/.ssh",
		"/var/h/ann/.config/authama": "~/.config/authama",
		"/private/var/h/anne/.ssh":   "/private/var/h/anne/.ssh", // a sibling, not the home
		"/etc/ssh":                   "/etc/ssh",
		"relative/var/h/ann/x":       "relative/var/h/ann/x",
		"":                           "",
	} {
		if got := tildePath(filepath.FromSlash(in), homes); got != filepath.FromSlash(want) {
			t.Errorf("tildePath(%q) = %q, want %q", in, got, want)
		}
	}
	text := "home: /private/var/h/ann/.config (config)\nssh:  /var/h/ann/.ssh\nother: /private/var/h/anne/x and /private/var/h/ann"
	want := "home: ~/.config (config)\nssh:  ~/.ssh\nother: /private/var/h/anne/x and ~"
	if filepath.Separator == '/' {
		if got := tildeText(text, homes); got != want {
			t.Errorf("tildeText:\n got %q\nwant %q", got, want)
		}
	}
	if got := tildePath("/x", nil); got != "/x" {
		t.Errorf("no home: %q", got)
	}
}
