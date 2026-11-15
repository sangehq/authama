package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/dataset"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/scheduler"
)

// newExportScheduleCmd is `export schedule`: saved selections written to a
// folder every day, by a job this system's scheduler runs.
func newExportScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Write saved selections to a folder every day",
		Long: "A schedule writes a saved selection to a folder, in one format, as <name>.<extension>,\n" +
			"replacing the file each time. `schedule install` registers one daily job with this\n" +
			"system's scheduler (launchd, systemd or cron, schtasks) that runs `schedule run`, which\n" +
			"writes every schedule; `schedule run` can be run by hand too. Schedules are yours\n" +
			"alone, kept under the state home: a folder is a path on this machine.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newScheduleAddCmd(), newScheduleListCmd(), newScheduleRemoveCmd(), newScheduleRunCmd(),
		newScheduleInstallCmd(true), newScheduleInstallCmd(false))
	return cmd
}

// scheduleFolder makes folder absolute and refuses one inside ~/.ssh, or,
// in dev mode, outside the sandbox.
func scheduleFolder(p paths.Paths, folder string) (string, error) {
	abs, err := filepath.Abs(folder)
	if err != nil {
		return "", err
	}
	if insideDir(abs, p.SSHDir) {
		return "", fmt.Errorf("refusing %s: it is inside %s, which an export must leave as it is", abs, p.SSHDir)
	}
	if !p.Contains(abs) {
		return "", fmt.Errorf("refusing %s in dev mode: it is outside the sandbox under %s", abs, p.DevRoot)
	}
	return abs, nil
}

func newScheduleAddCmd() *cobra.Command {
	var format, folder string
	cmd := &cobra.Command{
		Use:   "add <selection>",
		Short: "Schedule a saved selection to be written to a folder every day",
		Long: "Schedule the saved selection <selection> to be written to --to every day, in --format\n" +
			"(csv by default; the SSH client formats are not scheduled). Adding the same selection\n" +
			"and format again replaces the folder.",
		Example: "  " + brand.Command + " export schedule add prd-hosts --to ~/Reports\n" +
			"  " + brand.Command + " export schedule add prd-hosts --format json --to ~/Reports",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if folder == "" {
				return errors.New("give --to <folder>")
			}
			abs, err := scheduleFolder(p, folder)
			if err != nil {
				return err
			}
			if _, err := savedSelection(p, args[0]); err != nil {
				return err
			}
			s := savedsel.Schedule{Selection: args[0], Format: format, Folder: abs}
			if err := savedsel.AddSchedule(p, s); err != nil {
				return err
			}
			file, _ := s.File()
			_, _ = fmt.Fprintf(c.OutOrStdout(), "scheduled %s daily to %s; the daily job writes it once `%s export schedule install` has registered one\n",
				args[0], file, brand.Command)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "csv", "one of "+strings.Join(encode.TableNames(), ", "))
	cmd.Flags().StringVar(&folder, "to", "", "the folder to write to (made, mode 0700, if it is not there)")
	return cmd
}

func newScheduleListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List the export schedules",
		Example: "  " + brand.Command + " export schedule list",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			all, err := savedsel.Schedules(p)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if len(all) == 0 {
				_, _ = fmt.Fprintf(out, "No export schedules. Add one with `%s export schedule add <selection> --to <folder>`.\n", brand.Command)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "SELECTION\tFORMAT\tFILE")
			for _, s := range all {
				file, _ := s.File()
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Selection, s.Format, file)
			}
			return tw.Flush()
		},
	}
}

func newScheduleRemoveCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "remove <selection>",
		Short: "Stop writing a saved selection to its folder",
		Long: "Remove the schedules of <selection>: every format, or only --format. The files already\n" +
			"written stay where they are.",
		Example: "  " + brand.Command + " export schedule remove prd-hosts\n" +
			"  " + brand.Command + " export schedule remove prd-hosts --format json",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			n, err := savedsel.RemoveSchedules(p, args[0], format)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "removed %d schedule(s) of %s; the files written stay\n", n, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "", "only the schedule in this format")
	return cmd
}

func newScheduleRunCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Write every scheduled export now",
		Long: "Write every schedule's file now, as the daily job does. A schedule that fails is\n" +
			"reported and the rest still run; the command fails if any did.",
		Example: "  " + brand.Command + " export schedule run",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			all, err := savedsel.Schedules(p)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				m = nil
			}
			failed := 0
			for _, s := range all {
				file, rows, err := runSchedule(p, m, s)
				if err != nil {
					failed++
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "%s (%s): %v\n", s.Selection, s.Format, err)
					continue
				}
				if !quiet {
					_, _ = fmt.Fprintf(c.OutOrStdout(), "wrote %d row(s) to %s\n", rows, file)
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d scheduled export(s) failed", failed, len(all))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "print nothing unless one fails")
	return cmd
}

// runSchedule writes one schedule's file, answering where and how many rows.
func runSchedule(p paths.Paths, m *manifest.Manifest, s savedsel.Schedule) (string, int, error) {
	if err := savedsel.CheckSchedule(s); err != nil {
		return "", 0, err
	}
	if _, err := scheduleFolder(p, s.Folder); err != nil {
		return "", 0, err
	}
	saved, err := savedsel.Get(p, m, s.Selection)
	if err != nil {
		return "", 0, err
	}
	ds, err := dataset.Build(p, dataset.Options{Parts: selection.Parts(saved.Selection.Grain)})
	if err != nil {
		return "", 0, err
	}
	res, err := selection.Run(ds, saved.Selection)
	if err != nil {
		return "", 0, err
	}
	f, err := encode.Get(s.Format)
	if err != nil {
		return "", 0, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf, encode.Table{Columns: res.Columns, Rows: res.Rows},
		encode.Options{Title: brand.Command + " " + s.Selection}); err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(s.Folder, 0o700); err != nil {
		return "", 0, err
	}
	file, err := s.File()
	if err != nil {
		return "", 0, err
	}
	return file, len(res.Rows), fs.WriteTextAtomic(file, buf.String(), 0o600)
}

// newScheduleInstallCmd is `schedule install` (install) or `schedule
// uninstall`: the daily job that runs `schedule run`.
func newScheduleInstallCmd(install bool) *cobra.Command {
	use, short, example := "uninstall", "Remove the daily job that writes the scheduled exports", " export schedule uninstall"
	if install {
		use, short, example = "install", "Register a daily job that writes the scheduled exports", " export schedule install"
	}
	return &cobra.Command{
		Use:     use,
		Short:   short,
		Example: "  " + brand.Command + example,
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if !install {
				scheduler.Remove(scheduler.ExportsLabel)
				_, _ = fmt.Fprintln(c.OutOrStdout(), "removed the daily scheduled-exports job")
				return nil
			}
			if p.IsDev() {
				return errors.New("installing a schedule is refused in dev mode: it registers a job with " +
					"launchd, systemd or schtasks, which is machine-wide and outlives the sandbox")
			}
			command := scheduler.ExportsCommand()
			if err := scheduler.Install(command, scheduler.ExportsLabel); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "installed a daily job (%s): %s\n", scheduler.ExportsLabel, command)
			return nil
		},
	}
}
