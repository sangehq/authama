package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/repair"
	"github.com/simtabi/authama/src/internal/util/log"
)

// newRepairCmd is `authama repair`: find what is wrong and fix what can be
// fixed without guessing.
func newRepairCmd() *cobra.Command {
	var check, yes bool
	var areas []string
	cmd := &cobra.Command{
		Use:   "repair",
		Short: "Find and fix what is wrong with the authama home, ~/.ssh and the token files",
		Long: "Checks three areas and fixes what can be fixed safely:\n\n" +
			"  tool    missing config and state directories; a history database that fails\n" +
			"          its integrity check (renamed aside, never deleted); a manifest or\n" +
			"          inventory that does not load, and the owner seal (reported only)\n" +
			"  ssh     files a crashed command left; modes ssh or authama refuse; keys with\n" +
			"          no .pub; a ~/.ssh/config that no longer matches the manifest\n" +
			"  tokens  credential files others can read; a tokens.json that does not parse\n" +
			"          (reported only)\n\n" +
			"It lists what it found, asks, then fixes. --check lists only, and exits non-zero\n" +
			"when anything is wrong. Nothing is deleted.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			var want []repair.Area
			for _, a := range areas {
				ok := false
				for _, known := range repair.Areas {
					if a == string(known) {
						want, ok = append(want, known), true
					}
				}
				if !ok {
					return fmt.Errorf("unknown area %q: choose tool, ssh or tokens", a)
				}
			}
			env, err := tokenEnv(c)
			if err != nil {
				return err
			}
			h := repair.Home{P: p, Tokens: &env, EmitUseKeychain: platform.EmitUseKeychain()}
			found := h.Check(want...)
			out := c.OutOrStdout()
			if len(found) == 0 {
				say(c, "✓ nothing to repair")
				return nil
			}
			fixable := 0
			for _, f := range found {
				mark := "manual"
				if f.Fixable {
					mark, fixable = "fix", fixable+1
				}
				_, _ = fmt.Fprintf(out, "%-6s [%s] %s\n         -> %s\n", mark, f.Area, f.Problem, f.Fix)
			}
			if check {
				return fmt.Errorf("%d problem(s) found", len(found))
			}
			if fixable == 0 {
				return errors.New("nothing here can be fixed automatically; follow the steps above")
			}
			if err := confirmChange(c, fmt.Sprintf("Fix the %d marked fix?", fixable), yes); err != nil {
				return err
			}
			snapshotBeforeMutation(p)
			err = repair.Fix(found, out)
			log.Audit(p.AuditLog(), "repair", log.Field{Key: "fixed", Value: fmt.Sprint(fixable)})
			return err
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "list what is wrong and change nothing; exit non-zero if anything is")
	cmd.Flags().StringSliceVar(&areas, "area", nil, "only these areas: tool, ssh, tokens (repeatable)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// newResetCmd is `authama reset`: put authama back to before it was set up,
// moving its homes aside rather than deleting them.
func newResetCmd() *cobra.Command {
	var confirmWord string
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Factory reset: move the authama home aside (kept) and start again; ~/.ssh is not touched",
		Long: "Renames the config home, and the state home when it is separate, to\n" +
			"<name>.reset-<time>, so renaming them back undoes it. Nothing is deleted, and\n" +
			"~/.ssh - keys, config, known_hosts - is not touched. A home bound to its owner\n" +
			"is unbound first (its key in the OS store is removed), so a restored home is\n" +
			"never bound to a key that no longer exists. With a PIN set, it asks for it.\n\n" +
			"It asks you to type \"reset\"; a script passes --confirm reset.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			plan := repair.PlanFactoryReset(p, time.Now())
			if len(plan.Moves) == 0 {
				say(c, "nothing to reset: no config home exists")
				return nil
			}
			out := c.OutOrStdout()
			for _, mv := range plan.Moves {
				_, _ = fmt.Fprintf(out, "move  %s\n  ->  %s\n", mv[0], mv[1])
			}
			if plan.Unbinds {
				_, _ = fmt.Fprintln(out, "unbind the owner first: the PIN and the sealing key go")
			}
			_, _ = fmt.Fprintln(out, "~/.ssh is not touched.")
			if confirmWord == "" {
				if !platform.StdinIsTerminal() {
					return errors.New("a reset needs --confirm reset when there is no terminal to ask on")
				}
				_, _ = fmt.Fprint(out, `Type "reset" to go on: `)
				line, err := platform.ReadLine(c.InOrStdin())
				if err != nil {
					return err
				}
				confirmWord = strings.TrimSpace(line)
			}
			if confirmWord != "reset" {
				return errAborted
			}
			// The lock is taken to be sure no other authama is mid-change, and
			// let go before the move: its file is inside the state home.
			mutation.LockForMutation(p)
			mutation.FinishEarly()
			mutation.Release()
			if _, err := repair.FactoryReset(p, time.Now()); err != nil {
				return err
			}
			say(c, "✓ reset; `%s init` starts again, and renaming the folders back undoes it", brand.Command)
			return nil
		},
	}
	cmd.Flags().StringVar(&confirmWord, "confirm", "", `type "reset" here to skip the question`)
	return cmd
}
