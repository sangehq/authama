package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/events"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/notifier"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/desktop"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/scheduler"
)

// newStructureCmd is `ssh structure`: the watcher that finds keys outside the
// expected layout and suggests moves, which only the owner approves.
func newStructureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "structure",
		Short: "Find keys outside the expected layout and move them, once you approve each move",
		Long: "Watches ~/.ssh and the config home for keys that are not where authama expects them -\n" +
			"a pair loose in ~/.ssh, a key in the wrong profile directory, a key dropped in the\n" +
			"config home - and suggests a move for each. Nothing moves until you approve it, here\n" +
			"or on the web structure page. An approved move is previewed first, backs up each\n" +
			"file it moves (owner-only, in the state home's .state/moved/), declares the key in\n" +
			"its profile, records it, repoints IdentityFile lines in your hand-written ssh config,\n" +
			"re-renders the managed config, and is recorded in history.\n\n" +
			"`" + brand.Command + " serve` checks after every file change it sees; `install` adds\n" +
			"a background job that checks every 15 minutes and notifies about new suggestions.",
	}
	cmd.AddCommand(newStructureListCmd(), newStructureCheckCmd(), newStructureClassifyCmd(), newStructureApproveCmd(),
		newStructureDismissCmd(), newStructureRulesCmd(), newStructureInstallCmd(), newStructureUninstallCmd(),
		newStructureDiagramCmd())
	return cmd
}

func newStructureDiagramCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagram",
		Short: "Draw ~/.ssh as a tree: each folder and key, which hosts use a key, who owns a tool key",
		Long: "Draws ~/.ssh as it is on disk: the files OpenSSH reads, each category's config, every\n" +
			"profile's keys (both halves on one line as name{,.pub}) and the shared tools/ folder, each\n" +
			"line labelled from the manifest, then the counts. Dot-files, sockets and the old/ rotation\n" +
			"archives are left out. It only reads.",
		Example: "  " + brand.Command + " ssh structure diagram",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			d, err := structure.Diagram(p)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(c.OutOrStdout(), d.String())
			return err
		},
	}
}

func newStructureListCmd() *cobra.Command {
	var asJSON, all bool
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List the suggested moves (dismissed ones with --all)",
		Example: "  " + brand.Command + " ssh structure list\n  " + brand.Command + " ssh structure list --all --json",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			ss, _, err := structure.Check(p, time.Now())
			if err != nil {
				return err
			}
			if !all {
				ss = undismissed(ss)
			}
			if asJSON {
				if ss == nil {
					ss = []structure.Suggestion{}
				}
				return writeJSONOut(c, ss)
			}
			printSuggestions(c.OutOrStdout(), ss)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the suggestions as JSON")
	cmd.Flags().BoolVar(&all, "all", false, "include dismissed suggestions")
	return cmd
}

func newStructureCheckCmd() *cobra.Command {
	var notify, quiet bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Record file changes and refresh the suggestions; what the background job runs",
		Long: "Scans for file changes (the same scan as `" + brand.Command + " events`), refreshes the\n" +
			"suggestions and, with --notify, posts one desktop notification naming the new ones.\n" +
			"It never moves anything unless a purpose rule you enabled with --auto matches a complete suggestion (leftover deletes are never auto-applied).",
		Example: "  " + brand.Command + " ssh structure check --notify",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			now := time.Now()
			if _, err := events.Scan(p, eventRoots(p), now.UTC()); err != nil && !quiet {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "events scan: %v\n", err)
			}
			all, fresh, err := structure.Check(p, now)
			if err != nil {
				return err
			}
			if ids, err := change.AutoApplyRuled(p, all, c.OutOrStdout()); err != nil {
				return err
			} else if len(ids) > 0 {
				all, fresh, err = structure.Check(p, now)
				if err != nil {
					return err
				}
				if !quiet {
					say(c, "auto-applied %d purpose rule(s): %s", len(ids), strings.Join(ids, ", "))
				}
			}
			if notify && len(fresh) > 0 && notifier.DesktopEnabled() {
				if desktop.Notify(brand.Name+": keys outside the layout", notifyText(fresh)) {
					_ = structure.MarkNotified(p, fresh, now)
				}
			}
			if !quiet {
				open := undismissed(all)
				say(c, "%d suggestion(s), %d new", len(open), len(fresh))
				if len(open) > 0 {
					say(c, "  `%s ssh structure list` shows them", brand.Command)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&notify, "notify", false, "post a desktop notification about new suggestions")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print nothing")
	return cmd
}

func notifyText(fresh []structure.Suggestion) string {
	names := make([]string, 0, 3)
	for i, s := range fresh {
		if i == 3 {
			names = append(names, fmt.Sprintf("and %d more", len(fresh)-3))
			break
		}
		names = append(names, s.Path)
	}
	return strings.Join(names, ", ") + " - review with `" + brand.Command + " ssh structure list` or the web structure page. Nothing moves until you approve."
}

func newStructureApproveCmd() *cobra.Command {
	var profile, name, planFile string
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "approve [<id>...]",
		Short: "Preview, then carry out, the moves you approve",
		Long: "Approves suggestions by id (or an id prefix of 4+ characters), or a plan of move lines\n" +
			"with --plan (- reads stdin):\n\n" +
			"  key <~/.ssh/path|config/name> profile <profile> [name <key>]\n" +
			"  key <profile>/<key> profile <profile> [name <key>]   (a declared key, with its hosts)\n" +
			"  host <profile>/<alias> profile <profile>\n" +
			"  misplaced <profile>/<key>\n" +
			"  profile <profile> category <category|->\n\n" +
			"A host moved to another profile takes its own key when no other host uses it;\n" +
			"otherwise it takes the key its new profile gives it. A declared key moves only\n" +
			"with every host of its profile that uses it, and keeps its record and deployments.\n\n" +
			"A suggestion with no guessed profile needs --profile. The preview lists each file\n" +
			"moved and the manifest and config diffs; nothing is written before you confirm.",
		Example: "  " + brand.Command + " ssh structure approve 1a2b3c4d\n" +
			"  " + brand.Command + " ssh structure approve 9f8e --profile work --name laptop\n" +
			"  " + brand.Command + " ssh structure approve --plan moves.txt --dry-run\n" +
			"  echo \"host work/gh profile home\" | " + brand.Command + " ssh structure approve --plan -",
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			moves, err := approvedMoves(c, p, args, planFile, profile, name)
			if err != nil {
				return err
			}
			ch := change.StructurePlan(moves)
			plan, err := change.Preview(p, ch)
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			_, _ = fmt.Fprintln(w, "preview - nothing is written until you confirm:")
			printChangePlan(w, plan)
			if plan.Err != "" {
				return errors.New(plan.Err)
			}
			if dryRun {
				return nil
			}
			if err := confirmChange(c, "", yes); err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			if err := ch.OnApply(p, w); err != nil {
				return err
			}
			if err := ch.Run(p, w); err != nil {
				return err
			}
			_, _, _ = structure.Check(p, time.Now())
			say(c, "done; `%s history` lists it, `%s ssh snapshots` has the ~/.ssh state from before", brand.Command, brand.Command)
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "the profile to move the key into (for a suggestion with none guessed, or to override the guess)")
	cmd.Flags().StringVar(&name, "name", "", "the key's name in that profile (one id only; default: its file name)")
	cmd.Flags().StringVar(&planFile, "plan", "", "approve these move lines instead of ids (- reads stdin)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the preview; move nothing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "move without confirming (implied when there is no terminal)")
	return cmd
}

// approvedMoves turns ids (with --profile/--name) or a plan file into moves.
func approvedMoves(c *cobra.Command, p paths.Paths, ids []string, planFile, profile, name string) ([]structure.Move, error) {
	if planFile != "" {
		if len(ids) > 0 {
			return nil, errors.New("give ids or --plan, not both")
		}
		var b []byte
		var err error
		if planFile == "-" {
			b, err = io.ReadAll(c.InOrStdin())
		} else {
			b, err = os.ReadFile(planFile)
		}
		if err != nil {
			return nil, err
		}
		return structure.ParsePlan(string(b))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("name the suggestions to approve (see `%s ssh structure list`), or give --plan", brand.Command)
	}
	if name != "" && len(ids) != 1 {
		return nil, errors.New("--name names one key: approve one id with it")
	}
	all, err := structure.Suggest(p)
	if err != nil {
		return nil, err
	}
	found, missing := structure.Find(all, ids)
	if len(missing) > 0 {
		return nil, fmt.Errorf("no single suggestion has id %s (see `%s ssh structure list`)", strings.Join(missing, ", "), brand.Command)
	}
	var lines []string
	for _, s := range found {
		switch {
		case s.Info:
			return nil, fmt.Errorf("%s (%s) has nothing to approve: %s", s.ID, s.Path, s.Detail)
		case s.Kind == structure.KindMisplaced:
			lines = append(lines, s.Plan)
		case profile != "" || name != "":
			prof := profile
			if prof == "" {
				prof = s.Profile
			}
			if prof == "" {
				return nil, fmt.Errorf("%s (%s): name a profile with --profile", s.ID, s.Path)
			}
			key := name
			if key == "" {
				key = s.Name
			}
			lines = append(lines, structure.KeyLine(s.Path, prof, key))
		case s.NeedsProfile:
			return nil, fmt.Errorf("%s (%s): no profile could be guessed; name one with --profile", s.ID, s.Path)
		default:
			lines = append(lines, s.Plan)
		}
	}
	return structure.ParsePlan(strings.Join(lines, "\n"))
}

func newStructureDismissCmd() *cobra.Command {
	var undo bool
	cmd := &cobra.Command{
		Use:     "dismiss <id>...",
		Short:   "Hide suggestions you mean to leave as they are (--undo shows them again)",
		Example: "  " + brand.Command + " ssh structure dismiss 1a2b3c4d\n  " + brand.Command + " ssh structure dismiss 1a2b3c4d --undo",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			all, err := structure.Suggest(p)
			if err != nil {
				return err
			}
			found, missing := structure.Find(all, args)
			if len(missing) > 0 {
				return fmt.Errorf("no single suggestion has id %s", strings.Join(missing, ", "))
			}
			ids := make([]string, 0, len(found))
			for _, s := range found {
				ids = append(ids, s.ID)
			}
			if err := structure.Dismiss(p, ids, undo, time.Now()); err != nil {
				return err
			}
			verb := "dismissed"
			if undo {
				verb = "restored"
			}
			say(c, "%s %s", verb, strings.Join(ids, ", "))
			return nil
		},
	}
	cmd.Flags().BoolVar(&undo, "undo", false, "show the suggestions again")
	return cmd
}

func newStructureInstallCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Check in the background every 15 minutes and notify about new suggestions",
		Long: "Registers a job with launchd (macOS), a systemd user timer (or cron) on Linux, or a\n" +
			"Scheduled Task on Windows, that runs `ssh structure check --notify --quiet`. It only\n" +
			"suggests; moves still need your approval.",
		Example: "  " + brand.Command + " ssh structure install",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if err := refuseInDevMode(p, "ssh structure install",
				"it registers a job with launchd, systemd or schtasks, which is machine-wide and outlives the sandbox"); err != nil {
				return err
			}
			command := scheduler.StructureCommand()
			if err := confirmChange(c, "ssh structure install registers a job that runs every "+
				fmt.Sprint(scheduler.StructureEveryMinutes)+" minutes:\n  "+command, yes); err != nil {
				return err
			}
			lockForMutation(p)
			if err := scheduler.Install(command, scheduler.StructureLabel); err != nil {
				return err
			}
			say(c, "installed the structure check (%s): %s", scheduler.StructureLabel, command)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "install without confirming (implied when there is no terminal)")
	return cmd
}

func newStructureUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall",
		Short:   "Remove the background structure check",
		Example: "  " + brand.Command + " ssh structure uninstall",
		Args:    cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if err := refuseInDevMode(p, "ssh structure uninstall", "the job is machine-wide"); err != nil {
				return err
			}
			scheduler.Remove(scheduler.StructureLabel)
			say(c, "removed the structure check (%s), if it was installed", scheduler.StructureLabel)
			return nil
		},
	}
}

func undismissed(ss []structure.Suggestion) []structure.Suggestion {
	out := make([]structure.Suggestion, 0, len(ss))
	for _, s := range ss {
		if !s.Dismissed {
			out = append(out, s)
		}
	}
	return out
}

func printSuggestions(w io.Writer, ss []structure.Suggestion) {
	if len(ss) == 0 {
		_, _ = fmt.Fprintln(w, "everything is where authama expects it")
		return
	}
	for _, s := range ss {
		mark := ""
		if s.Dismissed {
			mark = " (dismissed)"
		}
		_, _ = fmt.Fprintf(w, "%s  %-11s %s%s\n", s.ID, s.Kind, s.Path, mark)
		_, _ = fmt.Fprintf(w, "          %s\n", s.Detail)
		switch {
		case s.Plan != "":
			to := ""
			if s.To != "" && s.To != s.Path {
				to = " -> " + s.To
			}
			_, _ = fmt.Fprintf(w, "          approve: %s ssh structure approve %s%s\n", brand.Command, s.ID, to)
		case s.NeedsProfile:
			_, _ = fmt.Fprintf(w, "          approve: %s ssh structure approve %s --profile <profile>\n", brand.Command, s.ID)
		}
		for _, f := range s.InUse {
			_, _ = fmt.Fprintf(w, "          named in %s (repointed on approval)\n", f)
		}
	}
}

// printChangePlan prints a change preview: its output, the effects, the
// files the copy holds no content for, and each diff.
func printChangePlan(w io.Writer, plan change.Plan) {
	if plan.Output != "" {
		_, _ = fmt.Fprint(w, plan.Output)
	}
	for _, e := range plan.Effects {
		_, _ = fmt.Fprintf(w, "* %s\n", e)
	}
	for _, h := range plan.Hidden {
		_, _ = fmt.Fprintf(w, "* %s\n", h)
	}
	for _, f := range plan.Files {
		_, _ = fmt.Fprintf(w, "--- %s (%s)\n", f.Path, f.State)
		_, _ = fmt.Fprint(w, history.FormatDiff(f.Lines))
	}
}
