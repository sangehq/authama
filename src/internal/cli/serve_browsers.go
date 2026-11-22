package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/services/browsers"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// browserRow is one remembered browser as `serve browsers --json` prints
// it: never its secret, nor the secret's hash.
type browserRow struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Created  time.Time `json:"created"`
	LastUsed time.Time `json:"last_used"`
	Expires  time.Time `json:"expires"`
}

// newServeBrowsersCmd is `authama serve browsers`: the browsers the web UI
// remembers, and forgetting them.
func newServeBrowsersCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "browsers",
		Short: "List the browsers the web UI remembers, or forget them",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			list, err := browsers.List(browsers.File(p.StateDir()), time.Now())
			if err != nil {
				return err
			}
			rows := make([]browserRow, 0, len(list))
			for _, b := range list {
				rows = append(rows, browserRow{b.ID, b.Label, b.Created.UTC(), b.LastUsed.UTC(), b.Expires.UTC()})
			}
			if asJSON {
				return writeJSONOut(c, rows)
			}
			out := c.OutOrStdout()
			if len(rows) == 0 {
				_, _ = fmt.Fprintln(out, "no browser is remembered; one is after it signs in with a link (run `authama`)")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tBROWSER\tSIGNED IN\tLAST USED\tEXPIRES")
			for _, r := range rows {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Label, tz.Minute(r.Created), tz.Minute(r.LastUsed), tz.Minute(r.Expires))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the list as JSON")

	var all bool
	forget := &cobra.Command{
		Use:   "forget [id...]",
		Short: "Forget remembered browsers: they sign in with a link again",
		RunE: func(c *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return errors.New("name the browsers to forget (see `authama serve browsers`), or give --all")
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			n, err := browsers.Forget(browsers.File(p.StateDir()), args, all, time.Now())
			if err != nil {
				return err
			}
			if !all && n < len(args) {
				return fmt.Errorf("forgot %d of %d; the others were not remembered (see `authama serve browsers`)", n, len(args))
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "forgot %d browser(s); a running web UI signs them out on their next page\n", n)
			return nil
		},
	}
	forget.Flags().BoolVar(&all, "all", false, "forget every remembered browser")
	cmd.AddCommand(forget)
	return cmd
}
