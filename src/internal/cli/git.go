package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/gitremote"
	"github.com/simtabi/authama/src/internal/util/envnames"
)

// gitModeEnv overrides defaults.git.mode for one command.
const gitModeEnv = envnames.GitMode

// resolveGitMode picks the mode: an explicit choice, then AUTHAMA_GIT_MODE, then
// defaults.git.mode, then profile.
func resolveGitMode(explicit string, m *manifest.Manifest) (string, error) {
	mode := explicit
	if mode == "" {
		mode = envnames.Get(nil, gitModeEnv)
	}
	if mode == "" {
		mode = m.Defaults.Git.EffectiveMode()
	}
	for _, known := range manifest.GitModes {
		if mode == known {
			return mode, nil
		}
	}
	return "", fmt.Errorf("mode %q is not one of %s", mode, strings.Join(manifest.GitModes, ", "))
}

func isGitMode(s string) bool {
	for _, known := range manifest.GitModes {
		if s == known {
			return true
		}
	}
	return false
}

func newGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Show or switch checkouts' GitHub remotes between profile SSH, plain SSH and HTTPS",
		Long: "Each checkout's origin reaches GitHub one of three ways:\n\n" +
			"  profile  git@github-<profile>:owner/repo.git  the profile's own key (default)\n" +
			"  ssh      git@github.com:owner/repo.git         the key bound to Host github.com\n" +
			"  https    https://github.com/owner/repo.git     your git credential helper\n\n" +
			"The owner picks the profile through defaults.git.orgs, then defaults.git.profile.",
	}

	var asJSON bool
	status := &cobra.Command{
		Use:   "status [path...]",
		Short: "Show each checkout's remote mode, owner and profile",
		Long: "Show how each checkout's origin reaches GitHub. With --json, prints\n" +
			"{\"version\": 1, \"default_mode\": ..., \"checkouts\": [...]}: each checkout's path,\n" +
			"state (managed, no-origin or unmanaged), url, mode, owner, repo, profile and\n" +
			"profile_from (remote or owner). Fields are only ever added.",
		RunE: func(c *cobra.Command, args []string) error {
			m, err := loadManifestFor(c)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				args = []string{"."}
			}
			rows := gitremote.Status(gitremote.Exec{}, m, args)
			out := c.OutOrStdout()
			if asJSON {
				return writeJSONTo(out, gitStatusDoc{Version: 1, DefaultMode: m.Defaults.Git.EffectiveMode(), Checkouts: rows})
			}
			for _, r := range rows {
				switch r.State {
				case gitremote.StateNoOrigin:
					_, _ = fmt.Fprintf(out, "%s  -  no origin\n", r.Path)
				case gitremote.StateUnmanaged:
					_, _ = fmt.Fprintf(out, "%s  -  not a managed GitHub remote (%s)\n", r.Path, r.URL)
				default:
					who := r.Profile
					switch {
					case who == "":
						who = "?"
					case r.ProfileFrom == gitremote.ProfileFromOwner:
						who += " (by owner)"
					}
					_, _ = fmt.Fprintf(out, "%s  %s  %s/%s  profile=%s\n", r.Path, r.Mode, r.Owner, r.Repo, who)
				}
			}
			return nil
		},
	}
	status.Flags().BoolVar(&asJSON, "json", false, "machine-readable output (scripting); fields are only ever added")
	cmd.AddCommand(status)

	var modeFlag, profileFlag string
	var dryRun, yes bool
	use := &cobra.Command{
		Use:   "use [profile|ssh|https] [path...]",
		Short: "Point checkouts' origin at a mode, after proving the new URL with ls-remote",
		Long: "Point each checkout's origin at a mode. The mode is the first argument when it\n" +
			"names one, else --mode, else $" + gitModeEnv + ", else defaults.git.mode, else profile.\n" +
			"A new URL is checked with git ls-remote before origin is rewritten, so a checkout\n" +
			"never ends up with a remote it cannot authenticate to. Non-GitHub origins are skipped.",
		RunE: func(c *cobra.Command, args []string) error {
			explicit := modeFlag
			if len(args) > 0 && isGitMode(args[0]) {
				explicit, args = args[0], args[1:]
			}
			m, err := loadManifestFor(c)
			if err != nil {
				return err
			}
			mode, err := resolveGitMode(explicit, m)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				args = []string{"."}
			}
			if !dryRun {
				if err := confirmChange(c, fmt.Sprintf("git use %s rewrites origin in %d checkout(s) after an ls-remote check.",
					mode, len(args)), yes); err != nil {
					return err
				}
				if p, err := resolvePaths(c); err == nil {
					lockForMutation(p)
				}
			}
			out := c.OutOrStdout()
			var failed []string
			for _, dir := range args {
				ch, err := gitremote.Use(gitremote.Exec{}, m, dir, mode, profileFlag, dryRun)
				switch {
				case err != nil:
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "%s: %v\n", dir, err)
					failed = append(failed, dir)
				case ch.Skipped != "":
					_, _ = fmt.Fprintf(out, "%s  skipped: %s\n", dir, ch.Skipped)
				case dryRun:
					_, _ = fmt.Fprintf(out, "%s  would change  %s -> %s\n", dir, ch.From, ch.To)
				default:
					_, _ = fmt.Fprintf(out, "%s  %s -> %s\n", dir, ch.From, ch.To)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d checkout(s) left unchanged: %s", len(failed), strings.Join(failed, ", "))
			}
			return nil
		},
	}
	use.Flags().StringVar(&modeFlag, "mode", "", "profile, ssh or https (overrides $"+gitModeEnv+" and the manifest)")
	use.Flags().StringVar(&profileFlag, "profile", "", "use this profile's key instead of the owner's mapping")
	use.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change, touch nothing")
	use.Flags().BoolVarP(&yes, "yes", "y", false, "rewrite without confirming (implied when there is no terminal)")
	cmd.AddCommand(use)
	return cmd
}

// gitStatusDoc is `ssh git status --json`. Other tools read it: add fields,
// never rename or remove one, and bump Version only for a change that breaks.
type gitStatusDoc struct {
	Version     int                  `json:"version"`
	DefaultMode string               `json:"default_mode"`
	Checkouts   []gitremote.Checkout `json:"checkouts"`
}

// loadManifestFor loads the manifest the command's paths point at.
func loadManifestFor(c *cobra.Command) (*manifest.Manifest, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return nil, err
	}
	return manifest.Load(p.Manifest())
}
