package cli

import (
	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/keysvc"
)

// newKeyAckCmd is `ssh key ack`: you have seen these keys, so they stop being
// labelled new until something else happens to them.
func newKeyAckCmd() *cobra.Command {
	var category string
	var all bool
	cmd := &cobra.Command{
		Use:   "ack [<profile/key>|profile|alias ...] [--category <name>] [--all]",
		Short: "Clear the new label on keys you have seen",
		Long: "A key is labelled new for keys.new_window_days (7 by default) after it is\n" +
			"created, added, rotated or deployed (keys.new_events chooses which). ack clears\n" +
			"the label now, and records when in key-events.json and the history. The next\n" +
			"of those events makes the key new again.",
		Example: "  " + brand.Command + " ssh key ack work/work_gh-ed25519\n" +
			"  " + brand.Command + " ssh key ack --all",
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if err := needsSelection(args, all, category); err != nil {
				return err
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			return change.KeyAck(keysvc.Selection{Selectors: args, Category: category, All: all}).Run(p, c.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "every key of the profiles filed under this category")
	cmd.Flags().BoolVar(&all, "all", false, "every key")
	return cmd
}
