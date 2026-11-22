package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/exports"
	"github.com/sangehq/authama/src/internal/services/query"
	"github.com/sangehq/authama/src/internal/util/csvout"
)

// newListCmd is the native list verb: profiles -> hosts (tree), filterable.
func newListCmd() *cobra.Command {
	var profile, provider, typ, tag string
	var asCSV bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Filterable tree across profiles",
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
			inv, err := inventory.Load(p.Inventory())
			if err != nil {
				return err
			}
			groups, err := query.New(m, inv, p.Providers()).Groups(profile, provider, typ, tag)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if asCSV {
				return csvout.Write(out, exports.Hosts(groups))
			}
			filtered := profile != "" || provider != "" || typ != "" || tag != ""
			if len(groups) == 0 && filtered {
				_, _ = fmt.Fprintln(out, "no hosts match the filter")
				return nil
			}
			writeHostTree(out, groups)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asCSV, "csv", false, "print the hosts as CSV, one row each, header first")
	cmd.Flags().StringVar(&profile, "profile", "", "filter to one profile")
	cmd.Flags().StringVar(&provider, "provider", "", "filter by provider")
	cmd.Flags().StringVar(&typ, "type", "", "provider category, e.g. vcs")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by host tag")
	return cmd
}

// writeHostTree prints each profile and, under it, its hosts in columns that
// line up across every profile: alias, hostname, [provider], key, (status),
// then the tags. No line ends in padding.
func writeHostTree(out io.Writer, groups []query.ProfileGroup) {
	cells := func(r query.HostRow) []string {
		tags := ""
		if len(r.Tags) > 0 {
			tags = "#" + strings.Join(r.Tags, " #")
		}
		return []string{r.Alias, r.Hostname, "[" + r.ProviderLabel + "]", r.KeyName, "(" + r.Status + ")", tags}
	}
	var widths []int
	for _, g := range groups {
		for _, r := range g.Rows {
			for i, c := range cells(r) {
				if i >= len(widths) {
					widths = append(widths, 0)
				}
				widths[i] = max(widths[i], utf8.RuneCountInString(c))
			}
		}
	}
	for _, g := range groups {
		if g.Empty {
			_, _ = fmt.Fprintf(out, "%s  (no hosts)\n", g.Name)
			continue
		}
		_, _ = fmt.Fprintln(out, g.Name)
		for _, r := range g.Rows {
			var line strings.Builder
			line.WriteString(" ")
			for i, c := range cells(r) {
				line.WriteString(" ")
				line.WriteString(c)
				line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c)+1))
			}
			_, _ = fmt.Fprintln(out, strings.TrimRight(line.String(), " "))
		}
	}
}
