package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
)

// docs/reference/cli.md is generated from the command tree, so every command
// and flag is documented and the page cannot drift from --help. After
// changing a command, regenerate it:
//
//	UPDATE_CLI_REFERENCE=1 go test ./internal/cli/ -run TestCLIReferenceIsCurrent
const cliReferencePath = "../../../docs/reference/cli.md"

func visibleChildren(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, s := range c.Commands() {
		if s.Hidden || s.Name() == "help" || s.Name() == "completion" || s.Deprecated != "" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func anchor(path string) string { return strings.ReplaceAll(path, " ", "-") }

func renderCLIReference(root *cobra.Command) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("# Command reference\n\n")
	w("<!-- Generated from the command tree by TestCLIReferenceIsCurrent in\n")
	w("     src/internal/cli/clireference_test.go. Do not edit by hand:\n")
	w("     UPDATE_CLI_REFERENCE=1 go test ./internal/cli/ -run TestCLIReferenceIsCurrent -->\n\n")
	w("Every `%s` command, with its flags and examples, exactly as `--help` prints\n", brand.Command)
	w("them. `%s` is the same program and accepts the same commands. The feature\n", brand.LegacyCommand)
	w("pages under [docs/tools](../tools/) explain what each one is for; this page is the\n")
	w("complete list. [Choosing between similar commands](#choosing-between-similar-commands)\n")
	w("is at the end.\n\n")
	w("Every command also takes the global flag:\n\n```\n%s```\n\n", root.PersistentFlags().FlagUsages())

	w("## Contents\n\n")
	var toc func(c *cobra.Command, depth int)
	toc = func(c *cobra.Command, depth int) {
		for _, s := range visibleChildren(c) {
			w("%s- [`%s`](#%s) - %s\n", strings.Repeat("  ", depth), s.CommandPath(), anchor(s.CommandPath()), s.Short)
			toc(s, depth+1)
		}
	}
	toc(root, 0)
	w("\n")

	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range visibleChildren(c) {
			w("## %s\n\n", s.CommandPath())
			if s.Long != "" {
				w("%s\n\n", strings.TrimSpace(s.Long))
			} else {
				w("%s.\n\n", strings.TrimSuffix(s.Short, "."))
			}
			w("```\n%s\n```\n\n", s.UseLine())
			if len(s.Aliases) > 0 {
				w("Aliases: `%s`\n\n", strings.Join(s.Aliases, "`, `"))
			}
			if kids := visibleChildren(s); len(kids) > 0 {
				w("Subcommands:\n\n")
				for _, k := range kids {
					w("- [`%s`](#%s) - %s\n", k.Name(), anchor(k.CommandPath()), k.Short)
				}
				w("\n")
			}
			if s.Example != "" {
				w("Examples:\n\n```sh\n%s\n```\n\n", strings.TrimRight(s.Example, "\n"))
			}
			if f := s.LocalNonPersistentFlags().FlagUsages(); strings.TrimSpace(f) != "" {
				w("Flags:\n\n```\n%s```\n\n", f)
			}
			walk(s)
		}
	}
	walk(root)
	w("%s", choosingSection)
	return b.String()
}

const choosingSection = `## Choosing between similar commands

Some names appear in both groups, or sound alike. Where a word appears in both
` + "`ssh`" + ` and ` + "`token`" + `, it means the same thing in both (a test in names_test.go keeps it that way).

| You want to… | Use | Not |
|---|---|---|
| see everything that is installed | ` + "`ssh list`" + ` (keys), ` + "`token list`" + ` (tokens) | |
| look at one profile, host or key in full | ` + "`ssh show NAME`" + ` | |
| look at one token in full | ` + "`token show TARGET HOST`" + ` | |
| see one host as ssh will connect to it | ` + "`ssh view HOST`" + ` (resolved config, key, deployment) | ` + "`ssh show`" + `, which lists files, blocks and pins |
| preview a change to ` + "`~/.ssh`" + ` | ` + "`ssh diff`" + `, or ` + "`ssh reconcile --dry-run`" + ` | ` + "`ssh config check`" + `, which only validates the config text |
| preview writing token files | ` + "`token apply --dry-run`" + ` | |
| a read-only overview that never fails | ` + "`token status`" + `, ` + "`ssh list`" + `, ` + "`ssh expiry`" + ` | |
| a check that fails CI when something is wrong | ` + "`doctor --strict`" + ` (both groups), ` + "`ssh config check`" + `, ` + "`ssh validate`" + `, ` + "`token audit`" + ` | |
| undo a key rotation | ` + "`ssh rollback`" + ` (the key's previous version) | ` + "`ssh restore`" + ` |
| put a whole machine back from a backup | ` + "`ssh restore`" + ` (an encrypted ` + "`ssh bundle`" + `) | ` + "`ssh rollback`" + ` |
| undo the last write to ` + "`~/.ssh`" + ` | ` + "`ssh snapshots restore`" + ` | |
| get back into a host after losing the key | ` + "`ssh recover`" + ` (prints a break-glass snippet) | |
| replace or rotate a token | ` + "`token set`" + ` again for the same host | |
| move an old config in | ` + "`token import`" + ` (tokens), ` + "`ssh import`" + ` (an existing ` + "`~/.ssh`" + `), ` + "`migrate`" + ` (an older layout of this tool) | |
`

func TestCLIReferenceIsCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got := renderCLIReference(newRootCmd())
	if os.Getenv("UPDATE_CLI_REFERENCE") != "" {
		if err := os.MkdirAll(filepath.Dir(cliReferencePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cliReferencePath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(cliReferencePath)
	if err != nil {
		t.Fatalf("%v; generate it with UPDATE_CLI_REFERENCE=1", err)
	}
	if string(want) != got {
		t.Fatalf("%s is stale: regenerate it with\n  UPDATE_CLI_REFERENCE=1 go test ./internal/cli/ -run TestCLIReferenceIsCurrent", cliReferencePath)
	}
}
