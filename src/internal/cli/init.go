package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/initsvc"
)

// newInitCmd is the native init verb: create/converge the per-user home.
func newInitCmd() *cobra.Command {
	var force, backup, noBackup, yes bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create/converge the per-user home",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			// --force resets manifest.json, inventory.json and .env, so it
			// keeps a copy of each by default and asks first at a terminal
			// (review R-BUG-02; owner, 2026-11-14). It reset them with no copy
			// and no question, and --backup was ignored without --force.
			stamp := ""
			if force {
				stamp = time.Now().Format("20060102-150405")
				where := "the current ones are copied to " + filepath.Join(p.StateDir(), "init-backup-"+stamp) + " first"
				if noBackup {
					where = "the current ones are NOT kept (--no-backup)"
				}
				if err := confirmChange(c, "init --force resets manifest.json, inventory.json and .env to their defaults; "+where+".", yes); err != nil {
					return err
				}
			}
			lockForMutation(p)
			res, err := initsvc.New(p, platform.EmitUseKeychain()).Run(force, force && !noBackup, stamp)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite manifest/inventory/.env with fresh defaults, keeping a copy of each in the state home's .state/ first")
	cmd.Flags().BoolVar(&noBackup, "no-backup", false, "with --force, keep no copy of the files it overwrites")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	// Deprecated: a backup is the default since 2026-11-14 (review R-BUG-02).
	// Still accepted, and still means a backup, so scripts that pass it keep
	// working. Remove no earlier than v0.2.0.
	cmd.Flags().BoolVar(&backup, "backup", false, "with --force, copy the old files into the state home's .state/ before overwriting")
	_ = cmd.Flags().MarkDeprecated("backup", "a backup is now taken by default; --no-backup skips it")
	return cmd
}
