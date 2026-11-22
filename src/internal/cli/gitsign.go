package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/gitremote"
	"github.com/sangehq/authama/src/internal/services/gitsign"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// newGitSignCmd is `ssh git sign`: each profile's git identity and SSH commit
// signing, rendered from the manifest's profiles.<name>.git.
func newGitSignCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Commit as each profile, and sign commits and tags with its SSH key",
		Long: "Each profile can carry a git identity (profiles.<name>.git in the manifest): the email\n" +
			"and name its commits are made as, the key that signs them (its github.com host's key\n" +
			"unless signing_key names another), and the directories whose checkouts are its own.\n\n" +
			"`apply` renders that into <config home>/git/<profile>.gitconfig, one allowed_signers\n" +
			"file, and one managed block of includeIf lines in the global git config, so every\n" +
			"checkout under a profile's directories commits and signs as that profile. `use` does\n" +
			"the same for one checkout wherever it is. `verify` checks recent commits' signatures.\n" +
			"No private key is read: git signs through ssh-keygen (or 1Password's op-ssh-sign).",
	}
	cmd.AddCommand(newGitSignSetCmd(), newGitSignApplyCmd(), newGitSignStatusCmd(),
		newGitSignUseCmd(), newGitSignVerifyCmd())
	return cmd
}

func newGitSignSetCmd() *cobra.Command {
	var email, name, key string
	var dirs []string
	var noSign, clear, noApply, yes bool
	cmd := &cobra.Command{
		Use:   "set <profile>",
		Short: "Set a profile's git email, name, signing key and directories, then apply",
		Long: "Sets profiles.<profile>.git in the manifest and applies it (see `sign apply`).\n" +
			"--dir is repeatable and replaces the list; give it once per directory whose\n" +
			"checkouts commit as this profile. Signing is on unless --no-sign. --clear removes\n" +
			"the profile's git block, and apply takes its includes out of the global config.",
		Example: "  " + brand.Command + " ssh git sign set work --email me@acme.example --name \"Ada Lovelace\" --dir ~/code/acme\n" +
			"  " + brand.Command + " ssh git sign set personal --email me@example.com --dir ~/code/personal --key github\n" +
			"  " + brand.Command + " ssh git sign set old-client --clear",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			prof, ok := m.Profiles[args[0]]
			if !ok {
				return fmt.Errorf("unknown profile: %q", args[0])
			}
			var g *manifest.ProfileGit
			if !clear {
				cur := manifest.ProfileGit{Sign: true}
				if prof.Git != nil {
					cur = *prof.Git
				}
				if c.Flags().Changed("email") {
					cur.Email = email
				}
				if c.Flags().Changed("name") {
					cur.Name = name
				}
				if c.Flags().Changed("key") {
					cur.SigningKey = key
				}
				if c.Flags().Changed("dir") {
					cur.Dirs = nil
					for _, d := range dirs {
						cur.Dirs = append(cur.Dirs, tildeHome(d))
					}
				}
				if c.Flags().Changed("no-sign") {
					cur.Sign = !noSign
				}
				g = &cur
			} else if prof.Git == nil {
				_, _ = fmt.Fprintf(c.OutOrStdout(), "%s has no git identity; nothing to clear\n", args[0])
				return nil
			}
			lockForMutation(p)
			if err := editor.New(p).SetProfileGit(args[0], g); err != nil {
				return err
			}
			verb := "set"
			if g == nil {
				verb = "cleared"
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "%s the git identity of profile %s\n", verb, args[0])
			if noApply {
				_, _ = fmt.Fprintf(c.OutOrStdout(), "run `%s ssh git sign apply` to write it\n", brand.Command)
				return nil
			}
			return applyGitSign(c, p, false, yes)
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "user.email, and the signer's principal in allowed_signers")
	cmd.Flags().StringVar(&name, "name", "", "user.name (empty leaves the global one)")
	cmd.Flags().StringVar(&key, "key", "", "the profile's key that signs (default: its github.com host's key)")
	cmd.Flags().StringArrayVar(&dirs, "dir", nil, "a directory whose checkouts commit as this profile (repeatable; replaces the list)")
	cmd.Flags().BoolVar(&noSign, "no-sign", false, "set the identity only, without signing")
	cmd.Flags().BoolVar(&clear, "clear", false, "remove the profile's git identity")
	cmd.Flags().BoolVar(&noApply, "no-apply", false, "change the manifest only")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "apply without confirming (implied when there is no terminal)")
	return cmd
}

// tildeHome writes a directory under the home directory as ~/…, the form the
// manifest keeps so it reads the same on another machine.
func tildeHome(d string) string {
	home, err := os.UserHomeDir()
	if err != nil || d == "" {
		return d
	}
	if abs, err := filepath.Abs(d); err == nil && !strings.HasPrefix(d, "~") {
		d = abs
	}
	if rel, err := filepath.Rel(home, d); err == nil && filepath.IsLocal(rel) {
		return "~/" + filepath.ToSlash(rel)
	}
	// Forward slashes on every system: git matches gitdir: with them, and a
	// backslash would end the config value early.
	return filepath.ToSlash(d)
}

func newGitSignApplyCmd() *cobra.Command {
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Write each profile's git config fragment, allowed_signers, and the includeIf block",
		Long: "Renders every profile's git block: <config home>/git/<profile>.gitconfig and\n" +
			"<config home>/git/allowed_signers (owner-only), and one managed block of includeIf\n" +
			"lines in the global git config ($GIT_CONFIG_GLOBAL, else ~/.gitconfig). Only that\n" +
			"block is ever changed, a copy of the file is kept under the state home's\n" +
			"backups/gitconfig/ first, and a symlinked ~/.gitconfig keeps its link. Every\n" +
			"change is shown as a diff before it is made.",
		Example: "  " + brand.Command + " ssh git sign apply --dry-run\n" +
			"  " + brand.Command + " ssh git sign apply -y",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			return applyGitSign(c, p, dryRun, yes)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the diff, write nothing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "write without confirming (implied when there is no terminal)")
	return cmd
}

// applyGitSign shows what an apply changes and makes it.
func applyGitSign(c *cobra.Command, p paths.Paths, dryRun, yes bool) error {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	pl, err := gitsign.Render(p, m, nil)
	if err != nil {
		return err
	}
	out := c.OutOrStdout()
	for _, prob := range pl.Problems {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s\n", prob)
	}
	if pl.UpToDate() {
		_, _ = fmt.Fprintln(out, "git identities are up to date")
		return nil
	}
	printGitSignPlan(out, pl)
	if dryRun {
		_, _ = fmt.Fprintln(out, "dry run: nothing written")
		return nil
	}
	if err := confirmChange(c, "", yes); err != nil {
		return err
	}
	lockForMutation(p)
	res, err := gitsign.Apply(p, pl, time.Now())
	if err != nil {
		return err
	}
	for _, w := range res.Written {
		_, _ = fmt.Fprintf(out, "wrote %s\n", w)
	}
	for _, r := range res.Removed {
		_, _ = fmt.Fprintf(out, "removed %s\n", r)
	}
	if res.Backup != "" {
		_, _ = fmt.Fprintf(out, "kept the global git config as it was in %s\n", res.Backup)
	}
	return nil
}

func printGitSignPlan(out io.Writer, pl gitsign.Plan) {
	for _, w := range pl.Pending() {
		state := "change"
		if w.Old == "" {
			state = "create"
		}
		_, _ = fmt.Fprintf(out, "--- %s (%s)\n", w.Path, state)
		_, _ = fmt.Fprint(out, history.FormatDiff(history.Diff(w.Old, w.New, 2)))
	}
	for _, r := range pl.Removes {
		_, _ = fmt.Fprintf(out, "--- %s (remove: its profile has no git identity now)\n", r)
	}
}

// gitSignStatusDoc is `ssh git sign status --json`; fields are only ever added.
type gitSignStatusDoc struct {
	Version   int                      `json:"version"`
	Global    string                   `json:"global_config"`
	UpToDate  bool                     `json:"up_to_date"`
	Profiles  []gitSignProfileRow      `json:"profiles"`
	Problems  []string                 `json:"problems"`
	Checkouts []gitsign.CheckoutStatus `json:"checkouts"`
}

type gitSignProfileRow struct {
	Profile string   `json:"profile"`
	Email   string   `json:"email"`
	Name    string   `json:"name"`
	Key     string   `json:"key"`
	Sign    bool     `json:"sign"`
	Signs   bool     `json:"signs"` // signing is on and the key exists
	Dirs    []string `json:"dirs"`
}

func newGitSignStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status [path...]",
		Short: "Show each profile's git identity, whether it is applied, and how checkouts commit",
		Long: "Lists the profiles with a git identity, says whether `apply` would change\n" +
			"anything, and for each path (default: the current directory, when it is a checkout)\n" +
			"the email, signing key and format git resolves there and the file the email comes from.",
		Example: "  " + brand.Command + " ssh git sign status\n" +
			"  " + brand.Command + " ssh git sign status ~/code/acme/app --json",
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			pl, err := gitsign.Render(p, m, nil)
			if err != nil {
				return err
			}
			doc := gitSignStatusDoc{Version: 1, Global: pl.Global, UpToDate: pl.UpToDate(),
				Profiles: []gitSignProfileRow{}, Problems: append([]string{}, pl.Problems...), Checkouts: []gitsign.CheckoutStatus{}}
			for _, s := range pl.Signers {
				doc.Profiles = append(doc.Profiles, gitSignProfileRow{Profile: s.Profile, Email: s.Git.Email, Name: s.Git.Name,
					Key: s.Key.String(), Sign: s.Git.Sign, Signs: s.Git.Sign && s.PubLine != "", Dirs: append([]string{}, s.Git.Dirs...)})
			}
			if len(args) == 0 && gitsign.IsCheckout(".") {
				args = []string{"."}
			}
			for _, dir := range args {
				doc.Checkouts = append(doc.Checkouts, gitsign.Status(dir))
			}
			out := c.OutOrStdout()
			if asJSON {
				return writeJSONTo(out, doc)
			}
			if len(doc.Profiles) == 0 {
				_, _ = fmt.Fprintf(out, "no profile has a git identity: set one with `%s ssh git sign set <profile> --email ...`\n", brand.Command)
			}
			for _, r := range doc.Profiles {
				sign := "identity only"
				switch {
				case r.Signs:
					sign = "signs with " + r.Key
				case r.Sign:
					sign = "signing waits for " + r.Key
				}
				dirs := "no directories (use `sign use` per checkout)"
				if len(r.Dirs) > 0 {
					dirs = strings.Join(r.Dirs, ", ")
				}
				_, _ = fmt.Fprintf(out, "%-14s %s  %s  %s\n", r.Profile, r.Email, sign, dirs)
			}
			for _, prob := range doc.Problems {
				_, _ = fmt.Fprintf(out, "problem: %s\n", prob)
			}
			if doc.UpToDate {
				_, _ = fmt.Fprintf(out, "applied: yes (%s)\n", doc.Global)
			} else {
				_, _ = fmt.Fprintf(out, "applied: no - run `%s ssh git sign apply` (%s)\n", brand.Command, doc.Global)
			}
			for _, st := range doc.Checkouts {
				if !st.Checkout {
					_, _ = fmt.Fprintf(out, "%s  -  not a git checkout\n", st.Dir)
					continue
				}
				sign := "unsigned"
				if st.Sign {
					sign = "signs (" + st.Format + ")"
				}
				_, _ = fmt.Fprintf(out, "%s  %s  %s  from %s\n", st.Dir, orDash(st.Email), sign, orDash(st.From))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output (scripting); fields are only ever added")
	return cmd
}

func newGitSignUseCmd() *cobra.Command {
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "use <profile> [path...]",
		Short: "Make checkouts commit and sign as a profile, wherever they are",
		Long: "Adds the profile's fragment to each checkout's own config (include.path), in\n" +
			"place of another profile's, so the checkout commits as that profile whatever\n" +
			"directory it is in. Includes that are not authama's stay. Run `sign apply` first\n" +
			"so the fragment exists.",
		Example: "  " + brand.Command + " ssh git sign use work .\n" +
			"  " + brand.Command + " ssh git sign use personal ~/src/dotfiles --dry-run",
		Args: cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			profile, dirs := args[0], args[1:]
			prof, ok := m.Profiles[profile]
			if !ok {
				return fmt.Errorf("unknown profile: %q", profile)
			}
			if prof.Git == nil {
				return fmt.Errorf("profile %s has no git identity: set one with `%s ssh git sign set %s --email ...`", profile, brand.Command, profile)
			}
			fragment := gitsign.FragmentPath(p, profile)
			if _, err := os.Stat(fragment); err != nil && !dryRun {
				return fmt.Errorf("%s is not written yet: run `%s ssh git sign apply` first", fragment, brand.Command)
			}
			if len(dirs) == 0 {
				dirs = []string{"."}
			}
			if !dryRun {
				if err := confirmChange(c, fmt.Sprintf("%d checkout(s) will commit as %s (include.path = %s).", len(dirs), profile, fragment), yes); err != nil {
					return err
				}
				lockForMutation(p)
			}
			out := c.OutOrStdout()
			var failed []string
			for _, dir := range dirs {
				res, err := gitsign.Use(dir, fragment, gitsign.Dir(p), dryRun)
				switch {
				case err != nil:
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "%s: %v\n", dir, err)
					failed = append(failed, dir)
				case !res.Added && len(res.Replaced) == 0:
					_, _ = fmt.Fprintf(out, "%s  already commits as %s\n", dir, profile)
				default:
					verb := "commits as"
					if dryRun {
						verb = "would commit as"
					}
					extra := ""
					if len(res.Replaced) > 0 {
						extra = " (was " + strings.Join(res.Replaced, ", ") + ")"
					}
					_, _ = fmt.Fprintf(out, "%s  %s %s%s\n", dir, verb, profile, extra)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d checkout(s) left unchanged: %s", len(failed), strings.Join(failed, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change, touch nothing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "change without confirming (implied when there is no terminal)")
	return cmd
}

func newGitSignVerifyCmd() *cobra.Command {
	var n int
	var asJSON, strict bool
	cmd := &cobra.Command{
		Use:   "verify [path]",
		Short: "Check the signatures of a checkout's recent commits against allowed_signers",
		Long: "Checks the last -n commits (default 10) with git's own signature check, against\n" +
			"authama's allowed_signers. Exits non-zero when a signature is bad, and with --strict\n" +
			"when any commit is not a good signature from an allowed signer.",
		Example: "  " + brand.Command + " ssh git sign verify\n" +
			"  " + brand.Command + " ssh git sign verify ~/code/acme/app -n 50 --strict",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			allowed := gitsign.AllowedSignersPath(p)
			if _, err := os.Stat(allowed); err != nil {
				allowed = "" // the checkout's own setting, if it has one
			}
			commits, err := gitsign.Verify(dir, allowed, n)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			bad, notGood := 0, 0
			for _, cm := range commits {
				if cm.Status == "B" {
					bad++
				}
				if !cm.Good() {
					notGood++
				}
			}
			if asJSON {
				if err := writeJSONTo(out, struct {
					Version int              `json:"version"`
					Path    string           `json:"path"`
					Commits []gitsign.Commit `json:"commits"`
				}{1, dir, append([]gitsign.Commit{}, commits...)}); err != nil {
					return err
				}
			} else {
				for _, cm := range commits {
					short := cm.Hash
					if len(short) > 10 {
						short = short[:10]
					}
					signer := ""
					if cm.Signer != "" {
						signer = " by " + cm.Signer
					}
					_, _ = fmt.Fprintf(out, "%s  %s  %s%s  %s\n", short, cm.Status, gitsign.Meaning(cm.Status), signer, cm.Subject)
				}
				_, _ = fmt.Fprintf(out, "%d of %d commit(s) carry a good signature from an allowed signer\n", len(commits)-notGood, len(commits))
			}
			switch {
			case bad > 0:
				return fmt.Errorf("%d commit(s) have a BAD signature", bad)
			case strict && notGood > 0:
				return fmt.Errorf("%d commit(s) are not signed by an allowed signer", notGood)
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&n, "number", "n", 10, "how many recent commits to check")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail unless every checked commit is a good signature from an allowed signer")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output (scripting); fields are only ever added")
	return cmd
}

// newGitCloneCmd is `ssh git clone owner/repo [dir]`: clone over the URL the
// mode and profile give, and make the checkout commit as that profile.
func newGitCloneCmd() *cobra.Command {
	var modeFlag, profileFlag string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "clone <owner>/<repo> [dir]",
		Short: "Clone a GitHub repository through the right profile's SSH alias, committing as that profile",
		Long: "Resolves the URL as `resolve` does (the owner's profile, the default mode), clones\n" +
			"it, and when the profile has a git identity adds its fragment to the new checkout\n" +
			"(as `sign use` does), so the first commit is already made and signed as that profile.",
		Example: "  " + brand.Command + " ssh git clone acme/app ~/code/acme/app\n" +
			"  " + brand.Command + " ssh git clone me/dotfiles --profile personal --mode profile",
		Args: cobra.RangeArgs(1, 2),
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
			dir := repo
			if len(args) == 2 {
				dir = args[1]
			}
			out := c.OutOrStdout()
			_, _ = fmt.Fprintf(out, "clone %s into %s (profile %s, %s mode)\n", r.URL, dir, orDash(r.Profile), r.Mode)
			if dryRun {
				return nil
			}
			if _, err := os.Stat(dir); err == nil {
				return fmt.Errorf("%s already exists", dir)
			}
			git := exec.Command("git", "clone", "--", r.URL, dir)
			git.Stdout, git.Stderr = out, c.ErrOrStderr()
			git.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
			if err := git.Run(); err != nil {
				return fmt.Errorf("git clone %s: %w", r.URL, err)
			}
			if r.Profile == "" || m.Profiles[r.Profile].Git == nil {
				return nil
			}
			fragment := gitsign.FragmentPath(p, r.Profile)
			if _, err := os.Stat(fragment); err != nil {
				_, _ = fmt.Fprintf(out, "profile %s has a git identity that is not applied yet: run `%s ssh git sign apply`, then `%s ssh git sign use %s %s`\n",
					r.Profile, brand.Command, brand.Command, r.Profile, dir)
				return nil
			}
			if _, err := gitsign.Use(dir, fragment, gitsign.Dir(p), false); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s commits as %s\n", dir, r.Profile)
			return nil
		},
	}
	cmd.Flags().StringVar(&modeFlag, "mode", "", "profile, ssh or https (overrides $"+gitModeEnv+" and the manifest)")
	cmd.Flags().StringVar(&profileFlag, "profile", "", "use this profile instead of the owner's mapping")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the URL and directory, clone nothing")
	return cmd
}
