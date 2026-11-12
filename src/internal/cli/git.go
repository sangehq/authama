package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/gitremote"
	"github.com/simtabi/authama/src/internal/services/keysvc"
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
	var dryRun, yes, useJSON bool
	use := &cobra.Command{
		Use:   "use [profile|ssh|https] [path...]",
		Short: "Point checkouts' origin at a mode, after proving the new URL with ls-remote",
		Long: "Point each checkout's origin at a mode. The mode is the first argument when it\n" +
			"names one, else --mode, else $" + gitModeEnv + ", else defaults.git.mode, else profile.\n" +
			"A new URL is checked with git ls-remote before origin is rewritten, so a checkout\n" +
			"never ends up with a remote it cannot authenticate to. Non-GitHub origins are skipped.\n\n" +
			"With --json, prints {\"version\": 1, \"mode\": ..., \"dry_run\": ..., \"checkouts\": [...]}:\n" +
			"each checkout's path, from, to, applied, skipped and error. Fields are only ever added.",
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
			doc := gitUseDoc{Version: 1, Mode: mode, DryRun: dryRun, Checkouts: []gitUseRow{}}
			for _, dir := range args {
				ch, err := gitremote.Use(gitremote.Exec{}, m, dir, mode, profileFlag, dryRun)
				if useJSON {
					row := gitUseRow{Path: dir, From: ch.From, To: ch.To, Applied: ch.Applied, Skipped: ch.Skipped}
					if err != nil {
						row.Error = err.Error()
						failed = append(failed, dir)
					}
					doc.Checkouts = append(doc.Checkouts, row)
					continue
				}
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
			if useJSON {
				if err := writeJSONTo(out, doc); err != nil {
					return err
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
	use.Flags().BoolVar(&useJSON, "json", false, "machine-readable output (scripting); fields are only ever added")
	cmd.AddCommand(use)
	cmd.AddCommand(newGitResolveCmd())
	return cmd
}

// newGitResolveCmd is `ssh git resolve owner/repo`: how a repository would be
// reached, read-only. sange's identity resolver calls it for the alias and key.
func newGitResolveCmd() *cobra.Command {
	var modeFlag, profileFlag string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "resolve <owner>/<repo>",
		Short: "Show the profile, mode, URL, SSH alias and key a GitHub repository would use; changes nothing",
		Long: "Show how a GitHub repository would be reached, without a checkout and without changing\n" +
			"anything. The profile is --profile, else the owner's (defaults.git.orgs, then defaults.git.profile);\n" +
			"the mode is --mode, else $" + gitModeEnv + ", else defaults.git.mode, else profile.\n\n" +
			"With --json, prints {\"version\": 1, \"profile\", \"mode\", \"url\", \"hostAlias\",\n" +
			"\"keyFingerprint\"}, the names sange's companion contract uses. hostAlias and keyFingerprint are\n" +
			"empty in https mode, and keyFingerprint is empty for a key not made yet. Fields are only ever added.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			owner, repo, ok := strings.Cut(strings.TrimSuffix(args[0], ".git"), "/")
			if !ok || owner == "" || repo == "" || strings.Contains(repo, "/") {
				return fmt.Errorf("want owner/repo, got %q", args[0])
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			mode, err := resolveGitMode(modeFlag, m)
			if err != nil {
				return err
			}
			r, err := gitremote.Resolve(m, owner, repo, mode, profileFlag)
			if err != nil {
				return err
			}
			doc := gitResolveDoc{Version: 1, Profile: r.Profile, Mode: r.Mode, URL: r.URL, HostAlias: r.HostAlias}
			if r.Key != nil {
				inv, err := inventory.Load(p.Inventory())
				if err != nil {
					return err
				}
				if row, err := keysvc.New(m, inv, p.SSHDir).Row(*r.Key); err == nil {
					doc.KeyFingerprint = row.Fingerprint
				}
			}
			out := c.OutOrStdout()
			if asJSON {
				return writeJSONTo(out, doc)
			}
			_, _ = fmt.Fprintf(out, "profile:      %s\nmode:         %s\nurl:          %s\n", doc.Profile, doc.Mode, doc.URL)
			if doc.HostAlias != "" {
				fp := doc.KeyFingerprint
				if fp == "" {
					fp = "(not made yet)"
				}
				_, _ = fmt.Fprintf(out, "host alias:   %s\nkey:          %s  %s\n", doc.HostAlias, r.Key, fp)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&modeFlag, "mode", "", "profile, ssh or https (overrides $"+gitModeEnv+" and the manifest)")
	cmd.Flags().StringVar(&profileFlag, "profile", "", "use this profile instead of the owner's mapping")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output (scripting); fields are only ever added")
	return cmd
}

// gitStatusDoc is `ssh git status --json`. Other tools read it: add fields,
// never rename or remove one, and bump Version only for a change that breaks.
type gitStatusDoc struct {
	Version     int                  `json:"version"`
	DefaultMode string               `json:"default_mode"`
	Checkouts   []gitremote.Checkout `json:"checkouts"`
}

// gitUseDoc is `ssh git use --json`. The same promise as gitStatusDoc.
type gitUseDoc struct {
	Version   int         `json:"version"`
	Mode      string      `json:"mode"`
	DryRun    bool        `json:"dry_run"`
	Checkouts []gitUseRow `json:"checkouts"`
}

// gitUseRow is one checkout in gitUseDoc: what changed, or would, or why not.
type gitUseRow struct {
	Path    string `json:"path"`
	From    string `json:"from"`
	To      string `json:"to"`
	Applied bool   `json:"applied"`
	Skipped string `json:"skipped"`
	Error   string `json:"error"`
}

// gitResolveDoc is `ssh git resolve --json`. Its names after version are the
// ones sange's companion contract (v1) gives, so they are camelCase. The same
// promise as gitStatusDoc.
type gitResolveDoc struct {
	Version        int    `json:"version"`
	Profile        string `json:"profile"`
	Mode           string `json:"mode"`
	URL            string `json:"url"`
	HostAlias      string `json:"hostAlias"`
	KeyFingerprint string `json:"keyFingerprint"`
}

// loadManifestFor loads the manifest the command's paths point at.
func loadManifestFor(c *cobra.Command) (*manifest.Manifest, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return nil, err
	}
	return manifest.Load(p.Manifest())
}
