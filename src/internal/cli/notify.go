package cli

import (
	"fmt"
	"slices"

	notifykit "github.com/simtabi/go-notifykit/src"
	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/notifier"
	"github.com/sangehq/authama/src/internal/util/desktop"
	"github.com/sangehq/authama/src/internal/util/scheduler"
)

// newNotifyCmd is the native notify verb group: install the scheduled expiry
// notifier, fire a test desktop notification, and the send, ping and doctor
// verbs every Simtabi Go program shares through go-notifykit.
func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Manage the scheduled expiry notifier and desktop notifications",
	}
	for _, v := range []struct{ verb, short string }{
		{"send", "Show a desktop notification from this process"},
		{"ping", "Hand a notification to the running web UI (authama serve), which shows it"},
	} {
		verb := v.verb
		cmd.AddCommand(&cobra.Command{
			Use:   verb + " [-title T] [-subtitle S] [-body B] [-icon PATH] [-open URL|PATH] [-group G] [-level info|warn|alert] [-sound] [-json]",
			Short: v.short,
			Long: v.short + ".\n\n" +
				"The flags are go-notifykit's, written with one dash: -title (or -message) is the first\n" +
				"line, -subtitle the second, -body (or -description) the text under them; -icon is\n" +
				"an absolute path to a PNG, JPEG or GIF; -open is a local http(s) URL or a program\n" +
				"to run on click; -group replaces an earlier notification in the group; -level is\n" +
				"info, warn or alert; -sound plays the default sound; -json reads one notification\n" +
				"as JSON from stdin instead.",
			Example:            "  " + brand.Command + " ssh notify " + verb + " -title \"Keys rotated\" -body \"3 keys\" -level info",
			DisableFlagParsing: true,
			RunE: func(c *cobra.Command, args []string) error {
				// Flag parsing is off so notifykit reads its own flags; --help
				// still means help here, never a notification.
				if slices.ContainsFunc(args, func(a string) bool { return a == "-h" || a == "--help" || a == "-help" }) {
					return c.Help()
				}
				return runNotifykit(c, append([]string{verb}, args...))
			},
		})
	}
	// doctor takes no flags, so cobra parses them: an unknown one is refused
	// instead of being ignored, and its help does not list send's flags.
	cmd.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Say whether desktop notifications work here, and why not",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runNotifykit(c, []string{"doctor"})
		},
	})

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
			// A notifier a release before 0.1.2 installed under the old label
			// would fire beside this one.
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
				_, _ = fmt.Fprintln(c.ErrOrStderr(), "no notification backend found: on Linux install notify-send (libnotify); on macOS and Windows, `authama notify doctor` says what is missing.")
			}
			return nil
		},
	})

	return cmd
}

// schedulerExe is the quoted invocation for the scheduled job; see scheduler.Exe.
func schedulerExe() string { return scheduler.Exe() }

// runNotifykit runs one go-notifykit verb. RunCLI says what went wrong
// itself; authama's exit contract is 0 or 1, so any failure is the silent
// errNotClean.
func runNotifykit(c *cobra.Command, args []string) error {
	n, err := desktop.NewNotifier()
	if err != nil {
		return err
	}
	if notifykit.RunCLI(c.Context(), n, args, c.InOrStdin(), c.OutOrStdout(), c.ErrOrStderr()) != 0 {
		return errNotClean
	}
	return nil
}
