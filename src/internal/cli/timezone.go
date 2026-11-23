package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// newSettingsTimezoneCmd is `authama settings timezone`: the zone every time
// is shown in. Not `authama config timezone`: `config` was the SSH config
// verb before the groups, and old scripts may still say it.
func newSettingsTimezoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timezone",
		Short: "Show, set or list the time zone times are shown in",
		Args:  cobra.ArbitraryArgs,
		RunE:  groupHelp,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show the zone times are shown in, and the time there now",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%s (now %s)\n", tz.Describe(), tz.Format(time.Now(), "2006-01-02 15:04:05 MST"))
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "set <zone>",
		Short: "Show times in an IANA zone (Europe/Berlin, UTC), or local for this system's",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			zone := strings.TrimSpace(args[0])
			if _, err := tz.Load(zone); err != nil {
				return err
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			if strings.EqualFold(zone, "local") || zone == "" {
				return change.SettingsUnset(zoneSetting).Run(p, c.OutOrStdout())
			}
			return change.SettingsSet(zoneSetting, zone).Run(p, c.OutOrStdout())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "list [search]",
		Short: "List the IANA zones, or those matching a search",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			q := ""
			if len(args) == 1 {
				q = args[0]
			}
			zones := tz.Search(q)
			if len(zones) == 0 {
				return errors.New("no zone matches " + q)
			}
			now := time.Now()
			for _, z := range zones {
				l, _ := time.LoadLocation(z)
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%-32s %s\n", z, now.In(l).Format("15:04 MST"))
			}
			return nil
		},
	})
	return cmd
}

// zoneSetting is the setting's dotted key.
const zoneSetting = "display.timezone"
