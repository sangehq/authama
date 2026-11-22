package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/core/recency"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/util/applog"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// logFollowEvery is how often `logs --follow` looks for new records.
var logFollowEvery = 500 * time.Millisecond

// logFilterFlags are the flags `logs` and `logs export` share.
type logFilterFlags struct {
	level, since, grep string
	limit              int
}

func (lf *logFilterFlags) add(c *cobra.Command, limit int) {
	c.Flags().StringVar(&lf.level, "level", "", "the least severe level to show: debug, info, warn or error")
	c.Flags().StringVar(&lf.since, "since", "", "records from then on: 36h, 7d, 2026-11-20, or RFC 3339")
	c.Flags().StringVar(&lf.grep, "grep", "", "records containing this text (any case)")
	c.Flags().IntVar(&lf.limit, "limit", limit, "the newest N records (0 for all)")
}

func (lf logFilterFlags) filter(now time.Time) (applog.Filter, error) {
	f := applog.Filter{Level: applog.ParseLevel(lf.level), Grep: lf.grep, Limit: lf.limit}
	if lf.level != "" && !applog.ValidLevel(lf.level) {
		return f, fmt.Errorf("--level %q: use debug, info, warn or error", lf.level)
	}
	if lf.level == "" {
		f.Level = applog.ParseLevel("debug")
	}
	if lf.since != "" {
		t, err := recency.ParseSince(lf.since, now)
		if err != nil {
			return f, err
		}
		f.Since = t
	}
	if lf.limit < 0 {
		return f, errors.New("--limit must be 0 or more")
	}
	return f, nil
}

// printEntry writes one record: its time in the display zone, level,
// message and attributes, or the record itself under --json.
func printEntry(w io.Writer, e applog.Entry, asJSON bool) {
	if asJSON {
		_, _ = fmt.Fprintln(w, e.Raw)
		return
	}
	keys := make([]string, 0, len(e.Attrs))
	for k := range e.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v, _ := json.Marshal(e.Attrs[k])
		s := string(v)
		if str, ok := e.Attrs[k].(string); ok && !strings.ContainsAny(str, " \t\"=") {
			s = str
		}
		fmt.Fprintf(&b, " %s=%s", k, s)
	}
	_, _ = fmt.Fprintf(w, "%s %-5s %s%s\n", tz.Format(e.Time, "2006-01-02 15:04:05 MST"), strings.ToUpper(e.Level), e.Msg, b.String())
}

// newLogsCmd is `authama logs`: authama.log, read, followed and exported.
func newLogsCmd() *cobra.Command {
	var (
		lf             logFilterFlags
		follow, asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show authama's log: commands, web actions, jobs, file changes and sign-ins",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			f, err := lf.filter(time.Now())
			if err != nil {
				return err
			}
			dir := p.LogDir()
			out := c.OutOrStdout()
			entries, err := applog.Read(dir, f)
			if err != nil {
				return err
			}
			for _, e := range entries {
				printEntry(out, e, asJSON)
			}
			if !follow {
				if len(entries) == 0 && !asJSON {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "no log records match (log: %s)\n", filepath.Join(dir, applog.FileName))
				}
				return nil
			}
			var offset int64
			if fi, err := os.Stat(filepath.Join(dir, applog.FileName)); err == nil {
				offset = fi.Size()
			}
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt)
			defer signal.Stop(stop)
			f.Limit, f.Since = 0, time.Time{}
			tick := time.NewTicker(logFollowEvery)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return nil
				case <-c.Context().Done():
					return nil
				case <-tick.C:
					var more []applog.Entry
					more, offset, err = applog.Tail(dir, offset, f)
					if err != nil {
						return err
					}
					for _, e := range more {
						printEntry(out, e, asJSON)
					}
				}
			}
		},
	}
	lf.add(cmd, 200)
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing new records until Ctrl-C")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print each record as its JSON line")

	var (
		ef  logFilterFlags
		dst string
	)
	export := &cobra.Command{
		Use:   "export",
		Short: "Write the log records (JSON lines) to a file, or stdout",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			f, err := ef.filter(time.Now())
			if err != nil {
				return err
			}
			entries, err := applog.Read(p.LogDir(), f)
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			if dst != "" && dst != "-" {
				file, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if err != nil {
					return err
				}
				defer func() { _ = file.Close() }()
				w = file
			}
			for _, e := range entries {
				_, _ = fmt.Fprintln(w, e.Raw)
			}
			if dst != "" && dst != "-" {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "wrote %d record(s) to %s\n", len(entries), dst)
			}
			return nil
		},
	}
	ef.add(export, 0)
	export.Flags().StringVarP(&dst, "out", "o", "", "the file to write (new; never overwritten), or - for stdout")
	cmd.AddCommand(export)

	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print where authama.log is",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), filepath.Join(p.LogDir(), applog.FileName))
			return nil
		},
	})
	cmd.AddCommand(logSettingCmd("level", "<debug|info|warn|error>", "Set the least severe level written to the log",
		"log.level", func(v string) error {
			if !applog.ValidLevel(v) {
				return fmt.Errorf("%q: use debug, info, warn or error", v)
			}
			return nil
		}))
	cmd.AddCommand(logSettingCmd("retention", "<days>", "Set how many days rotated log files are kept",
		"log.retention_days", positiveInt))
	return cmd
}

func positiveInt(v string) error {
	if n, err := strconv.Atoi(v); err != nil || n < 1 {
		return fmt.Errorf("%q: give a whole number of days, 1 or more", v)
	}
	return nil
}

// logSettingCmd is a `logs` subcommand that writes one log.* setting.
func logSettingCmd(use, arg, short, key string, check func(string) error) *cobra.Command {
	return &cobra.Command{
		Use:   use + " " + arg,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			v := strings.ToLower(strings.TrimSpace(args[0]))
			if err := check(v); err != nil {
				return err
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			return change.SettingsSet(key, v).Run(p, c.OutOrStdout())
		},
	}
}
