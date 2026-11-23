package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
)

// Every visible command explains itself beyond its one-line summary, and
// every entry in commandLong names a command that exists and has no Long of
// its own (a stale entry would never show).
func TestEveryCommandHasADescription(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := newRootCmd()
	for k := range commandLong {
		c, _, err := root.Find(strings.Fields(k))
		if err != nil || c.CommandPath() != brand.Command+" "+k {
			t.Errorf("commandLong[%q] names no command", k)
		} else if c.Long != commandLong[k] {
			t.Errorf("%s sets its own Long, so commandLong[%q] never shows", c.CommandPath(), k)
		}
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range visibleChildren(c) {
			if strings.TrimSpace(s.Long) == "" {
				t.Errorf("%s has only its one-line summary: add a description to commandLong or its Long", s.CommandPath())
			}
			walk(s)
		}
	}
	walk(root)
}
