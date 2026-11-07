package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/notifier"
	"github.com/simtabi/authama/src/internal/util/scheduler"
)

// newNotifyCmd is the native notify verb group: install the scheduled expiry
// notifier, or fire a test desktop notification.
func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Manage the scheduled expiry notifier",
	}

	var installYes bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Install the scheduled expiry notifier",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if err := refuseInDevMode(p, "notify install",
				"it registers a job with launchd, systemd or schtasks, which is "+
					"machine-wide and outlives the sandbox"); err != nil {
				return err
			}
			command := schedulerExe() + " ssh audit --notify"
			if err := confirmChange(c,
				"notify install registers a scheduled job with this system's scheduler:\n  "+
					command, installYes); err != nil {
				return err
			}
			lockForMutation(p)
			if err := scheduler.Install(command, scheduler.Label); err != nil {
				return err
			}
			// A job a release before 0.1.2 installed under the old label would
			// fire beside this one.
			scheduler.Remove(scheduler.LegacyLabel)
			_, _ = fmt.Fprintf(c.OutOrStdout(), "installed scheduled notifier: %s\n", command)
			return nil
		},
	}
	install.Flags().BoolVarP(&installYes, "yes", "y", false, "install without confirming (implied when there is no terminal)")
	cmd.AddCommand(install)

	cmd.AddCommand(&cobra.Command{
		Use:   "test",
		Short: "Fire a test notification",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			if notifier.New(p, m).Test() {
				_, _ = fmt.Fprintln(c.OutOrStdout(), "sent a test desktop notification.")
			} else {
				_, _ = fmt.Fprintln(c.ErrOrStderr(), "no notification backend found (install notify-send / terminal-notifier).")
			}
			return nil
		},
	})

	return cmd
}

// schedulerExe is the quoted invocation for the scheduled job; see scheduler.Exe.
func schedulerExe() string { return scheduler.Exe() }
