package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
)

// helpTemplate lays every command's help out as a manual page does: NAME,
// SYNOPSIS, DESCRIPTION, then the commands, options, examples and where to
// read more, each section headed in capitals and indented under it. The same
// sections, in the same order, are what `authama man` writes as troff.
const helpTemplate = `NAME
    {{.CommandPath}} - {{.Short}}

SYNOPSIS
    {{synopsis .}}{{if .HasAvailableSubCommands}}
    {{.CommandPath}} <command> [options]{{end}}
{{with (or .Long .Short)}}
DESCRIPTION
{{indent . 4}}
{{end}}{{if gt (len .Aliases) 0}}
ALIASES
    {{.NameAndAliases}}
{{end}}{{if .HasAvailableSubCommands}}
COMMANDS
{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}    {{rpad .Name .NamePadding }} {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
OPTIONS
{{indent (trimTrailingWhitespaces .LocalFlags.FlagUsages) 2}}
{{end}}{{if .HasAvailableInheritedFlags}}
GLOBAL OPTIONS
{{indent (trimTrailingWhitespaces .InheritedFlags.FlagUsages) 2}}
{{end}}{{if .HasExample}}
EXAMPLES
{{indent (trimLeft .Example) 4}}
{{end}}
SEE ALSO
    {{seeAlso .}}
`

// indent prefixes every non-empty line of s with n spaces.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

// seeAlso names the parent command, the help and man commands, and the
// manual's hosted copy.
func seeAlso(c *cobra.Command) string {
	var refs []string
	if c.HasParent() {
		refs = append(refs, c.Parent().CommandPath())
	}
	if c.HasAvailableSubCommands() {
		refs = append(refs, c.CommandPath()+" help <command>")
	}
	refs = append(refs, "man "+manName(c),
		"https://opensource.simtabi.com/documentation/sangehq/"+brand.Command+"/")
	return strings.Join(refs, ", ")
}

// synopsis is the usage line in a manual's words: options, not flags.
func synopsis(c *cobra.Command) string {
	return strings.Replace(c.UseLine(), "[flags]", "[options]", 1)
}

// manName is the manual page a command's help is also written to:
// authama-ssh-deploy for `authama ssh deploy`.
func manName(c *cobra.Command) string {
	return strings.ReplaceAll(c.CommandPath(), " ", "-")
}

func init() {
	cobra.AddTemplateFunc("indent", indent)
	cobra.AddTemplateFunc("seeAlso", seeAlso)
	cobra.AddTemplateFunc("synopsis", synopsis)
	cobra.AddTemplateFunc("trimLeft", func(s string) string {
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimLeft(l, " ")
		}
		return strings.Join(lines, "\n")
	})
}

// applyManStyle sets the manual-page help on the whole tree, and gives every
// list verb the Unix alias ls and every delete verb rm, as a shell user types
// them. Aliases only: no command a script names changes.
func applyManStyle(root *cobra.Command) {
	root.SetHelpTemplate(helpTemplate)
	root.SetUsageTemplate(helpTemplate)
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			switch sub.Name() {
			case "list":
				addAlias(sub, "ls")
			case "delete":
				addAlias(sub, "rm")
			}
			walk(sub)
		}
	}
	walk(root)
}

func addAlias(c *cobra.Command, alias string) {
	for _, a := range c.Aliases {
		if a == alias {
			return
		}
	}
	if c.HasParent() {
		for _, sib := range c.Parent().Commands() {
			if sib != c && (sib.Name() == alias || sib.HasAlias(alias)) {
				return // a sibling already answers to it
			}
		}
	}
	c.Aliases = append(c.Aliases, alias)
}
