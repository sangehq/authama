package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/exports"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/util/csvout"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// eventRoots is what a home's events track: ~/.ssh, the config home, and each
// tool's credential file.
func eventRoots(p paths.Paths) []events.Tracked {
	return events.Roots(p, tokensvc.UserFiles(tokensvc.NewEnv(p, nil, nil, "")))
}

func newEventsCmd() *cobra.Command {
	var since time.Duration
	var area, kind string
	var limit int
	var asJSON, asCSV, noScan, verify bool
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Every change seen to a file under ~/.ssh, the config home or a token file, newest first",
		Long: "authama tracks the files it is responsible for - everything under ~/.ssh, the config\n" +
			"home, and each tool's credential file - and records every change to them, whoever\n" +
			"made it: authama, an editor, a package manager. Each run scans first, then lists.\n" +
			"An event names the file and what changed (added, changed, removed, mode), never\n" +
			"its contents. Each line is chained to the one before it by a SHA-256 hash, so\n" +
			"--verify catches a line edited, removed or inserted after the fact, or a log\n" +
			"cut short. The web UI scans every 30 seconds while it runs.",
		Example: "  " + brand.Command + " events\n" +
			"  " + brand.Command + " events --since 24h --area auth\n" +
			"  " + brand.Command + " events --kind mode --json\n" +
			"  " + brand.Command + " events --verify",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if area != "" && area != events.AreaSSH && area != events.AreaConfig && area != events.AreaAuth {
				return fmt.Errorf("--area %q: want ssh, config or auth", area)
			}
			if verify {
				in, err := events.Verify(p)
				if err != nil {
					return err
				}
				out := c.OutOrStdout()
				if in.OK() {
					_, _ = fmt.Fprintf(out, "events log intact: %d line(s), %d chained", in.Lines, in.Chained)
					if in.Unchained > 0 {
						_, _ = fmt.Fprintf(out, ", %d from before the chain", in.Unchained)
					}
					_, _ = fmt.Fprintln(out)
					return nil
				}
				if in.Truncated {
					_, _ = fmt.Fprintf(out, "events log TAMPERED: %s\n", in.Reason)
				} else {
					_, _ = fmt.Fprintf(out, "events log TAMPERED at line %d: %s\n", in.Broken, in.Reason)
				}
				return errNotClean
			}
			if !noScan {
				if _, err := events.Scan(p, eventRoots(p), time.Now().UTC()); err != nil {
					return err
				}
			}
			f := events.Filter{Area: area, Kind: kind, Limit: limit}
			if since > 0 {
				f.Since = time.Now().Add(-since)
			}
			evs, err := events.List(p, f)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if asCSV {
				return csvout.Write(out, exports.Events(evs))
			}
			if asJSON {
				if evs == nil {
					evs = []events.Event{}
				}
				b, _ := json.MarshalIndent(evs, "", "  ")
				_, _ = fmt.Fprintln(out, string(b))
				return nil
			}
			if len(evs) == 0 {
				_, _ = fmt.Fprintln(out, "no events")
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "TIME\tKIND\tAREA\tFILE\tDETAIL")
			for _, e := range evs {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Time.Local().Format("2006-01-02 15:04:05"), e.Kind, orDashStr(e.Area), orDashStr(e.Path), e.Detail)
			}
			return tw.Flush()
		},
	}
	f := cmd.Flags()
	f.DurationVar(&since, "since", 0, "only events newer than this, e.g. 24h")
	f.StringVar(&area, "area", "", "only this area: ssh, config or auth")
	f.StringVar(&kind, "kind", "", "only this kind: added, changed, removed, mode or started")
	f.IntVar(&limit, "limit", 0, "at most this many (newest first); 0 is all")
	f.BoolVar(&asJSON, "json", false, "machine-readable output (scripting)")
	f.BoolVar(&asCSV, "csv", false, "one CSV row per event, header first")
	f.BoolVar(&noScan, "no-scan", false, "list what is recorded without scanning first")
	f.BoolVar(&verify, "verify", false, "check the log's hash chain for edited, removed or inserted lines; exit non-zero if broken")
	return cmd
}

func orDashStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
