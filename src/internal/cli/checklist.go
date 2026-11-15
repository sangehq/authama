package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/services/checklist"
)

// newChecklistCmd is `authama checklist`: for every key and token, where it is
// used and what is left to rotate or revoke it completely.
func newChecklistCmd() *cobra.Command {
	var asJSON, open bool
	c := &cobra.Command{
		Use:   "checklist",
		Short: "Where each key and token is used, and what is left to rotate or revoke it completely",
		Long: `A checklist per key and per token. While a key is in use it lists where the key is:
its files, the hosts that use it, each target it is installed on. Once it is revoked, or a
rotation replaces it, it lists what still has to go: targets it could not be removed from,
the private key, ssh-agent, encrypted bundles, and copies authama cannot see.

A token's revocation checklist starts with ` + "`authama checklist revoking <item>`" + `.

Steps authama can check it works out each time. The rest (the token's issuer, Termius, a
CI secret) you tick by hand with ` + "`authama checklist tick`" + `; ticks are kept in the state
directory. An item is named by its id from this list, or a key by profile/key.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			all, err := checklist.New(p).All()
			if err != nil {
				return err
			}
			if open {
				var keep []checklist.Checklist
				for _, cl := range all {
					if cl.Open() > 0 {
						keep = append(keep, cl)
					}
				}
				all = keep
			}
			if asJSON {
				return writeJSONOut(c, all)
			}
			out := c.OutOrStdout()
			if len(all) == 0 {
				_, err := fmt.Fprintln(out, "nothing to show: no key or token has steps left")
				return err
			}
			for _, cl := range all {
				done, total := cl.Counts()
				if _, err := fmt.Fprintf(out, "%-6s %-8s %2d/%-2d %-36s %s\n", cl.Kind, cl.Mode, done, total, cl.Name, cl.ID); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print every checklist as JSON")
	c.Flags().BoolVar(&open, "open", false, "only items with steps left")
	c.AddCommand(newChecklistShowCmd(), newChecklistTickCmd(true), newChecklistTickCmd(false), newChecklistRevokingCmd())
	return c
}

func newChecklistShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "show <item>",
		Short: "One item's checklist: every step, what is done, and the command for the rest",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			cl, err := checklist.New(p).Get(args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSONOut(c, cl)
			}
			_, err = fmt.Fprint(c.OutOrStdout(), checklist.Format(cl))
			return err
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print the checklist as JSON")
	return c
}

func newChecklistTickCmd(on bool) *cobra.Command {
	use, short := "tick <item> <step>", "Mark a step authama cannot check as done"
	if !on {
		use, short = "untick <item> <step>", "Mark a ticked step as not done"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long: short + `. The step is the id ` + "`authama checklist show`" + ` prints after "by hand:".
Only steps authama cannot see for itself can be ticked.`,
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			cl, err := checklist.New(p).Tick(args[0], args[1], on)
			if err != nil {
				return err
			}
			done, total := cl.Counts()
			verb := "ticked"
			if !on {
				verb = "unticked"
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%s %s on %s: %d of %d done\n", verb, args[1], cl.Name, done, total)
			return err
		},
	}
}

func newChecklistRevokingCmd() *cobra.Command {
	var stop bool
	c := &cobra.Command{
		Use:   "revoking <token item>",
		Short: "Start a token's revocation checklist (--stop to go back to where it is used)",
		Long: `A token has no revoked state of its own: authama cannot tell whether its issuer still
accepts it. This switches its checklist to the steps for revoking it - at the issuer, in each
tool's file, in authama's .env, in CI - and --stop switches it back.

A key's checklist follows the key; revoke it with ` + "`authama ssh key revoke`" + ` or rotate it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			cl, err := checklist.New(p).SetRevoking(args[0], !stop)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(c.OutOrStdout(), checklist.Format(cl))
			return err
		},
	}
	c.Flags().BoolVar(&stop, "stop", false, "stop the revocation checklist")
	return c
}

func writeJSONOut(c *cobra.Command, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(c.OutOrStdout(), strings.TrimSpace(string(b)))
	return err
}
