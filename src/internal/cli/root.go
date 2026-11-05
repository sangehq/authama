// Package cli is the cobra command surface. The binary is authama
// (brand.Command). sshmgr (brand.LegacyCommand), the name it shipped under
// before, is no longer built.
//
// Run on its own, on a terminal, it opens the web UI (as `authama serve`). The
// tree has two feature groups and a few commands that serve both:
//
//	authama ssh <verb>          SSH keys, profiles, hosts and config
//	authama token|auth <verb>   package-manager and git auth tokens
//	authama doctor|init|migrate|tui|history|serve|settings|version
//
// Every group answers `help [verb]`, as the root does.
//
// Every SSH verb is also kept, hidden, at the top level, so `authama deploy`
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

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/util/envnames"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/version"
)

// sshGroup is the name of the SSH feature group.
const sshGroup = "ssh"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   brand.Command,
		Short: brand.Name + " - " + brand.Summary,
		Long: brand.Name + " manages SSH keys, profiles, hosts and config, and the auth tokens package\n" +
			"managers and git read, from one manifest in the config home.\n\n" +
			"Run on its own, on a terminal, it opens the web UI on this machine, as\n" +
			brand.Command + " serve does. Give it a command for the command line: " + brand.Command + " ssh for\n" +
			"keys, profiles, hosts and config; " + brand.Command + " token (or auth) for auth tokens; " + brand.Command + " tui\n" +
			"for the menu; and the commands below that serve both. Every group answers\n" +
			"help <command>, and " + brand.Command + " man writes a manual page for every command.",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          rootArgs,
		// Bare `authama` on a terminal opens the web UI, exactly as `authama serve`
		// with its defaults; `authama <command>` is the CLI and `authama tui` the
		// menu. Someone who runs the binary by double-clicking it, or by typing its
		// name to see what it does, gets something they can use rather than a wall
		// of help.
		//
		// With no terminal it prints help instead: serve runs until Ctrl-C, so a
		// script that ran bare `authama` would hang rather than fail.
		RunE: func(c *cobra.Command, _ []string) error {
			if !bareIsInteractive() {
				return c.Help()
			}
			return runBare(c)
		},
	}
	root.SetVersionTemplate(brand.Command + " {{.Version}}\n")
	// A sandbox root for the whole run: --dev-root ./scratch puts the ssh dir and
	// the config home under it and touches nothing in $HOME. Persistent, because
	// it has to apply to every verb - a flag that only some commands honoured
	// would produce a run that reads the sandbox and writes the real tree.
	root.SetFlagErrorFunc(unknownCommandBeforeFlags)
	root.PersistentFlags().String(devRootFlag, "",
		"run against a sandbox directory instead of ~/.ssh (also $"+paths.DevRootEnv+
			", or the legacy $"+strings.Join(envnames.Legacy(paths.DevRootEnv), ", $")+")")
	root.PersistentFlags().Bool(noDotenvFlag, false,
		"do not read <config home>/.env this run (also $"+envnames.NoDotenv+"=1)")
	// Start-up - first-run migration, .env and authama.json - runs ahead of
	// every command, including those with a pre-run of their own (traversal
	// hooks: the root's runs first).
	cobra.EnableTraverseRunHooks = true
	root.PersistentPreRunE = startup

	// Commands that serve both features.
	root.AddCommand(newVersionCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newMigrateCmd())
	root.AddCommand(newTuiCmd())
	root.AddCommand(newHistoryCmd())
	root.AddCommand(newEventsCmd())
	root.AddCommand(newManCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newSettingsCmd())
	root.AddCommand(newOwnerCmd())
	root.AddCommand(newRepairCmd())
	root.AddCommand(newResetCmd())
	// `ui` was serve's alias, one edit from `tui`: still accepted, never shown.
	ui := newServeCmd()
	ui.Use, ui.Hidden = "ui", true
	ui.Deprecated = "use `" + brand.Command + " serve` (the web UI) or `" + brand.Command + " tui` (the terminal UI)"
	root.AddCommand(ui)

	// The two feature groups.
	ssh := &cobra.Command{
		Use:   sshGroup,
		Short: "SSH keys, profiles, hosts, config and trust",
		Long: "SSH keys, profiles, hosts, config and trust: everything authama does in ~/.ssh.\n\n" +
			"Every verb here also answers at the top level, unlisted, as it did before the\n" +
			"groups existed (" + brand.Command + " deploy is " + brand.Command + " ssh deploy), so older scripts keep working.",
		Args: cobra.ArbitraryArgs,
		RunE: groupHelp,
	}
	ssh.AddCommand(sshCommands()...)
	root.AddCommand(ssh)
	root.AddCommand(newTokenCmd())

	// The pre-group spellings, hidden: a second, identical build of each SSH
	// verb, so `authama deploy` keeps working without listing every verb twice.
	for _, c := range sshCommands() {
		c.Hidden = true
		root.AddCommand(c)
	}
	applyExamples(root, "")
	addGroupHelp(root)
	applyManStyle(root)
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

// bareIsInteractive is the terminal check behind a bare run; a test seam,
// since a test's stdin is never a terminal.
var bareIsInteractive = platform.StdinIsTerminal

// runBare runs `serve` as if it had been typed, with every serve flag at its
// default. The global flags (--dev-root, --no-dotenv) are the root's, so they
// apply to the bare run as they do to `authama serve`.
func runBare(root *cobra.Command) error {
	serve, _, err := root.Find([]string{"serve"})
	if err != nil || serve == root {
		return errors.New("this build has no serve command")
	}
	serve.SetContext(root.Context())
	return serve.RunE(serve, nil)
}

// addGroupHelp gives every command that has subcommands a `help [verb]` of
// its own, so `authama ssh help deploy` answers as `authama help ssh deploy`
// does. Cobra adds a help command to the root only; without this a group read
// "help" as an unknown verb. Leaf commands keep --help: some take a name as
// their argument, and a name may be "help".
func addGroupHelp(c *cobra.Command) {
	for _, sub := range c.Commands() {
		addGroupHelp(sub)
	}
	if !c.HasParent() || !c.HasSubCommands() {
		return
	}
	for _, sub := range c.Commands() {
		if sub.Name() == "help" {
			return
		}
	}
	group := c
	c.AddCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command in " + group.CommandPath(),
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			target, rest, err := group.Find(args)
			if err != nil || len(rest) > 0 {
				word := ""
				if len(rest) > 0 {
					word = rest[0]
				} else if len(args) > 0 {
					word = args[len(args)-1]
				}
				return unknownCommand(group, word)
			}
			return target.Help()
		},
	})
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
