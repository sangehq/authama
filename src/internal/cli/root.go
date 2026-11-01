// Package cli is the cobra command surface. The binary is authama
// (brand.Command) and is also installed as sshmgr (brand.LegacyCommand), the
// name v1 installed as its console script.
//
// The tree has two feature groups and a few commands that serve both:
//
//	authama ssh <verb>     SSH keys, profiles, hosts and config
//	authama token <verb>   package-manager and git auth tokens
//	authama doctor|init|migrate|tui|history|serve|version
//
// Every SSH verb is also kept, hidden, at the top level, so `sshmgr deploy`
// and every script and rendered ProxyCommand written before the groups
// existed still run. TestTheCommandNamesAreDistinct keeps the tree free of
// names a reader (or cobra) could confuse.
//
// The verb set is pinned against the implementation it replaced by
// TestTheCommandSurfaceMatchesTheOneItReplaced, which is what makes "no verb
// was lost" a checked claim rather than a recollection.
package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/ssh-manager/src/v3/internal/brand"
	"github.com/simtabi/ssh-manager/src/v3/internal/platform"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/paths"
	"github.com/simtabi/ssh-manager/src/v3/internal/version"
)

// sshGroup is the name of the SSH feature group.
const sshGroup = "ssh"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           brand.Command,
		Short:         brand.Name + " - " + brand.Summary,
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          rootArgs,
		// Bare `authama` on a terminal opens the menu; `authama <verb>` is the CLI.
		// Someone who runs the binary by double-clicking it, or by typing its name
		// to see what it does, gets something they can use rather than a wall of
		// help - and a script still gets the CLI, because a script has no terminal.
		//
		// Piped or redirected input falls back to help: a TUI reading from a pipe
		// would consume whatever is there as menu answers, which is how a
		// non-interactive run ends up picking a menu item nobody chose.
		RunE: func(c *cobra.Command, _ []string) error {
			if !platform.StdinIsTerminal() {
				return c.Help()
			}
			return runTUI(c)
		},
	}
	root.SetVersionTemplate(brand.Command + " {{.Version}}\n")
	// A sandbox root for the whole run: --dev-root ./scratch puts the ssh dir and
	// the config home under it and touches nothing in $HOME. Persistent, because
	// it has to apply to every verb - a flag that only some commands honoured
	// would produce a run that reads the sandbox and writes the real tree.
	root.SetFlagErrorFunc(unknownCommandBeforeFlags)
	root.PersistentFlags().String(devRootFlag, "",
		"run against a sandbox directory instead of ~/.ssh (also $"+paths.DevRootEnv+")")

	// Commands that serve both features.
	root.AddCommand(newVersionCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newMigrateCmd())
	root.AddCommand(newTuiCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newServeCmd())
	// `ui` was serve's alias, one edit from `tui`: still accepted, never shown.
	ui := newServeCmd()
	ui.Use, ui.Hidden = "ui", true
	ui.Deprecated = "use `" + brand.Command + " serve` (the web UI) or `" + brand.Command + " tui` (the terminal UI)"
	root.AddCommand(ui)

	// The two feature groups.
	ssh := &cobra.Command{
		Use:   sshGroup,
		Short: "SSH keys, profiles, hosts, config and trust (every former top-level sshmgr verb, still accepted there)",
		Args:  cobra.ArbitraryArgs,
		RunE:  groupHelp,
	}
	ssh.AddCommand(sshCommands()...)
	root.AddCommand(ssh)
	root.AddCommand(newTokenCmd())

	// The pre-group spellings, hidden: a second, identical build of each SSH
	// verb, so `sshmgr deploy` keeps working without listing every verb twice.
	for _, c := range sshCommands() {
		c.Hidden = true
		root.AddCommand(c)
	}
	applyExamples(root, "")
	recordCommands(root)
	return root
}

// sshCommands builds the SSH group's verbs. It is called twice: once for
// `ssh <verb>` and once for the hidden top-level compatibility spellings.
func sshCommands() []*cobra.Command {
	return []*cobra.Command{
		newConfigCmd(), newValidateCmd(), newProvidersCmd(), newNetCmd(),
		newSnapshotsCmd(), newKnownHostsCmd(), newReconcileCmd(), newKeygenCmd(),
		newLoadCmd(), newDiffCmd(), newProfileCmd(), newHostCmd(), newKeyCmd(),
		newImportCmd(), newRecoverCmd(), newBundleCmd(), newRestoreCmd(),
		newDeployCmd(), newRotateCmd(), newRollbackCmd(), newListCmd(),
		newViewCmd(), newShowCmd(), newCleanCmd(), newExpiryCmd(), newAuditCmd(),
		newNotifyCmd(), newGitCmd(), newVMCmd(), newTermiusCmd(),
	}
}

// groupHelp is a group's own action: help, or an unknown-verb error with a
// suggestion when a word was given.
func groupHelp(c *cobra.Command, args []string) error {
	if len(args) > 0 {
		return unknownCommand(c, args[0])
	}
	return c.Help()
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the " + brand.Command + " version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", brand.Command, version.Version)
			return nil
		},
	}
}

// unknownCommandBeforeFlags reports a word that is not a command ahead of a
// flag error. The root takes no arguments, so `authama nosuchcmd --open` is
// parsed as the root's own flags first, and cobra's "unknown flag: --open"
// hides the real problem: this build has no such command (an older binary
// on PATH, typically). A flag error on any real command is left as it is.
func unknownCommandBeforeFlags(c *cobra.Command, err error) error {
	if c.HasParent() || c.Flags().NArg() == 0 {
		return err
	}
	return unknownCommand(c, c.Flags().Arg(0))
}

// rootArgs refuses any word given to the root, as cobra.NoArgs did, but says
// which commands it may have meant: NoArgs reports an unknown command with no
// suggestion, so `authama termus` never pointed at `termius`.
func rootArgs(c *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return unknownCommand(c, args[0])
}

func unknownCommand(c *cobra.Command, word string) error {
	msg := fmt.Sprintf("unknown command %q for %q", word, c.Name())
	if c.SuggestionsMinimumDistance <= 0 {
		c.SuggestionsMinimumDistance = 2
	}
	s := c.SuggestionsFor(word)
	// The SSH verbs' top-level spellings are hidden, so cobra never suggests
	// them; suggest the grouped spelling instead.
	if !c.HasParent() {
		for _, g := range c.Commands() {
			if g.Name() == sshGroup {
				g.SuggestionsMinimumDistance = c.SuggestionsMinimumDistance
				for _, x := range g.SuggestionsFor(word) {
					s = append(s, sshGroup+" "+x)
				}
			}
		}
	}
	if len(s) > 0 {
		msg += "\n\nDid you mean this?\n\t" + strings.Join(s, "\n\t")
	}
	return errors.New(msg + "\n\nRun '" + c.CommandPath() + " --help' for the commands this build has (" +
		brand.Command + " version names it).")
}
