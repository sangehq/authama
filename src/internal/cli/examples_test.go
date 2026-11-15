package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
)

// splitExample splits one example line the way a POSIX shell would for the
// forms used in examples: words, "double quotes", 'single quotes', and a
// trailing # comment. A line that pipes into or evals the command is reduced
// to the authama invocation inside it.
func splitExample(line string) []string {
	if i := strings.Index(line, brand.Command+" "); i >= 0 {
		line = line[i+len(brand.Command)+1:]
	}
	line = strings.TrimSuffix(strings.TrimSpace(line), ")\"")
	var out []string
	var cur strings.Builder
	quote, in := rune(0), false
	for _, r := range line {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, in = r, true
		case r == '#' && !in:
			return out
		case r == ' ' || r == '\t':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out
}

// Every command in the reference has examples, and every example is a
// command line this binary accepts: the command exists, every flag is one it
// takes, and the positional arguments satisfy its Args rule. Nothing runs.
func TestEveryCommandHasExamplesThatParse(t *testing.T) {
	for k := range commandExamples {
		if c, _, err := newRootCmd().Find(strings.Fields(k)); err != nil || c.CommandPath() != brand.Command+" "+k {
			t.Errorf("commandExamples[%q] names no command", k)
		}
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range visibleChildren(c) {
			if len(visibleChildren(s)) == 0 && strings.TrimSpace(s.Example) == "" {
				t.Errorf("%s has no examples: add them to commandExamples or its Example", s.CommandPath())
			}
			// A trailing backslash continues the line, as in a shell.
			joined := strings.ReplaceAll(s.Example, "\\\n", " ")
			for _, line := range strings.Split(joined, "\n") {
				if strings.TrimSpace(line) == "" {
					continue
				}
				args := splitExample(line)
				root := newRootCmd()
				cmd, rest, err := root.Find(args)
				if err != nil || cmd.CommandPath() != s.CommandPath() {
					t.Errorf("%s: example %q runs %v, not this command (%v)", s.CommandPath(), line, cmd.CommandPath(), err)
					continue
				}
				if err := cmd.ParseFlags(rest); err != nil {
					t.Errorf("%s: example %q: %v", s.CommandPath(), line, err)
					continue
				}
				if cmd.Args != nil {
					if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
						t.Errorf("%s: example %q: %v", s.CommandPath(), line, err)
					}
				}
			}
			walk(s)
		}
	}
	walk(newRootCmd())
}
