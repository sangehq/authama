package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/version"
)

func newManCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "man [DIR]",
		Short: "Write a manual page (troff, section 1) for every command into DIR",
		Long: "Writes one manual page per command, " + brand.Command + ".1, " + brand.Command + "-ssh.1, " + brand.Command + "-ssh-deploy.1\n" +
			"and so on, with the sections the help prints: NAME, SYNOPSIS, DESCRIPTION,\n" +
			"COMMANDS, OPTIONS, EXAMPLES and SEE ALSO. DIR defaults to ./man; put it on\n" +
			"MANPATH, or copy the pages into a man1 directory, to read them with man(1).",
		Example: "  " + brand.Command + " man\n" +
			"  " + brand.Command + " man ~/.local/share/man/man1",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			dir := "man"
			if len(args) == 1 {
				dir = args[0]
			}
			n, err := writeManPages(c.Root(), dir, time.Now())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "wrote %d manual pages to %s\n", n, dir)
			return nil
		},
	}
}

// writeManPages writes a page for every visible command under root.
func writeManPages(root *cobra.Command, dir string, now time.Time) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	n := 0
	var walk func(c *cobra.Command) error
	walk = func(c *cobra.Command) error {
		if c.Hidden || c.Deprecated != "" || (c.Name() == "help" && c.HasParent()) || c.Name() == "completion" && c.HasParent() && !c.Parent().HasParent() {
			return nil
		}
		page := manPage(c, now)
		if err := os.WriteFile(filepath.Join(dir, manName(c)+".1"), []byte(page), 0o644); err != nil {
			return err
		}
		n++
		for _, sub := range c.Commands() {
			if err := walk(sub); err != nil {
				return err
			}
		}
		return nil
	}
	return n, walk(root)
}

// roff escapes text for a troff body line: backslashes, and a leading dot or
// quote that troff would read as a request.
func roff(s string) string {
	s = strings.ReplaceAll(s, `\`, `\e`)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, ".") || strings.HasPrefix(l, "'") {
			lines[i] = `\&` + l
		}
	}
	return strings.Join(lines, "\n")
}

func manPage(c *cobra.Command, now time.Time) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	name := manName(c)
	w(".TH %q 1 %q %q %q\n", strings.ToUpper(name), now.Format("2006-01-02"), brand.Command+" "+version.Version, brand.Name+" manual")
	w(".SH NAME\n%s \\- %s\n", roff(name), roff(c.Short))
	w(".SH SYNOPSIS\n.B %s\n", roff(synopsis(c)))
	if c.HasAvailableSubCommands() {
		w(".br\n.B %s <command> [options]\n", roff(c.CommandPath()))
	}
	desc := c.Long
	if desc == "" {
		desc = c.Short
	}
	w(".SH DESCRIPTION\n")
	for _, para := range strings.Split(desc, "\n\n") {
		w(".PP\n%s\n", roff(strings.ReplaceAll(para, "\n", " ")))
	}
	if len(c.Aliases) > 0 {
		w(".SH ALIASES\n%s\n", roff(strings.Join(append([]string{c.Name()}, c.Aliases...), ", ")))
	}
	if c.HasAvailableSubCommands() {
		w(".SH COMMANDS\n")
		subs := c.Commands()
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
		for _, s := range subs {
			if s.IsAvailableCommand() {
				w(".TP\n.B %s\n%s See \\fB%s\\fR(1).\n", roff(s.Name()), roff(s.Short), roff(manName(s)))
			}
		}
	}
	flags := func(title string, fs *pflag.FlagSet) {
		if !fs.HasAvailableFlags() {
			return
		}
		w(".SH %s\n", title)
		fs.VisitAll(func(f *pflag.Flag) {
			if f.Hidden {
				return
			}
			spell := "\\-\\-" + f.Name
			if f.Shorthand != "" {
				spell = "\\-" + f.Shorthand + ", " + spell
			}
			if f.Value.Type() != "bool" {
				spell += " " + strings.ToUpper(f.Value.Type())
			}
			w(".TP\n.B %s\n%s\n", spell, roff(f.Usage))
		})
	}
	flags("OPTIONS", c.NonInheritedFlags())
	flags("GLOBAL OPTIONS", c.InheritedFlags())
	if c.HasExample() {
		w(".SH EXAMPLES\n.nf\n%s\n.fi\n", roff(strings.TrimRight(c.Example, "\n")))
	}
	var see []string
	if c.HasParent() {
		see = append(see, "\\fB"+roff(manName(c.Parent()))+"\\fR(1)")
	}
	w(".SH SEE ALSO\n%s\n", strings.Join(append(see, brand.DocsURL), ", "))
	return b.String()
}
