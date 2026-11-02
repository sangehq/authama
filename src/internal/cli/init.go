package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/initsvc"
)

// newInitCmd is the native init verb: create/converge the per-user home.
func newInitCmd() *cobra.Command {
	var force, backup bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create/converge the per-user home",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			stamp := ""
			if force && backup {
				stamp = time.Now().Format("20060102-150405")
			}
			res, err := initsvc.New(p, platform.EmitUseKeychain()).Run(force, backup, stamp)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite manifest/inventory/.env with fresh defaults")
	cmd.Flags().BoolVar(&backup, "backup", false, "with --force, copy the old files into the state home's .state/ before overwriting")
	return cmd
}
