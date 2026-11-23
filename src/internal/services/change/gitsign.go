package change

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/gitsign"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// GitSignSet is `authama ssh git sign set <profile> …` followed by `sign
// apply`; with git nil it is `--clear`. GitSignApply is `sign apply` alone.
//
// The manifest edit runs in the preview's copy, so its diff is shown. The git
// files - each profile's fragment and allowed_signers in the config home, and
// the managed block in the global git config - are outside what a preview
// copies, so they are worked out from the manifest as it will be, listed in
// Effects line by line (the confirmation is bound to them), and written in
// OnApply only. A private key is never read: the fragments name a public key.
func GitSignSet(profile string, git *manifest.ProfileGit) Change {
	args := []string{profile}
	if git == nil {
		args = append(args, "--clear")
	} else {
		args = append(args, "--email="+git.Email)
		if git.Name != "" {
			args = append(args, "--name="+git.Name)
		}
		if git.SigningKey != "" {
			args = append(args, "--key="+git.SigningKey)
		}
		for _, d := range git.Dirs {
			args = append(args, "--dir="+d)
		}
		if !git.Sign {
			args = append(args, "--no-sign")
		}
	}
	args = append(args, "--yes")
	edit := func(m *manifest.Manifest) error {
		prof, ok := m.Profiles[profile]
		if !ok {
			return fmt.Errorf("unknown profile: %q", profile)
		}
		prof.Git = git
		m.SetProfile(profile, prof)
		return nil
	}
	return Change{
		Command: "ssh git sign set", Args: args,
		Subjects:   []history.Subject{{Kind: history.KindProfile, Name: profile}},
		NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			pl, err := gitSignPlan(p, edit)
			if err != nil {
				return nil, err
			}
			return gitSignEffects(pl), nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			pl, err := gitSignPlan(p, edit)
			if err != nil {
				return err
			}
			return applyGitSign(p, pl, out)
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := editor.New(p).SetProfileGit(profile, git); err != nil {
				return err
			}
			verb := "set"
			if git == nil {
				verb = "cleared"
			}
			_, _ = fmt.Fprintf(out, "%s the git identity of profile %s\n", verb, profile)
			return nil
		},
	}
}

// GitSignApply is `authama ssh git sign apply --yes`: the git files brought in
// line with the manifest as it is.
func GitSignApply() Change {
	return Change{
		Command: "ssh git sign apply", Args: []string{"--yes"}, NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			pl, err := gitSignPlan(p, nil)
			if err != nil {
				return nil, err
			}
			if pl.UpToDate() {
				return nil, errors.New("git identities are up to date: nothing to write")
			}
			return gitSignEffects(pl), nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			pl, err := gitSignPlan(p, nil)
			if err != nil {
				return err
			}
			return applyGitSign(p, pl, out)
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// GitSignUse is `authama ssh git sign use <profile> <dir>... --yes`: each
// checkout's own config includes the profile's fragment, in place of another
// profile's. It touches the checkouts only, on apply.
func GitSignUse(profile string, dirs []string) Change {
	args := append(append([]string{profile}, dirs...), "--yes")
	check := func(p paths.Paths) (string, error) {
		m, err := manifest.Load(p.Manifest())
		if err != nil {
			return "", err
		}
		prof, ok := m.Profiles[profile]
		switch {
		case !ok:
			return "", fmt.Errorf("unknown profile: %q", profile)
		case prof.Git == nil:
			return "", fmt.Errorf("profile %s has no git identity: set one first", profile)
		case len(dirs) == 0:
			return "", errors.New("name at least one checkout")
		}
		fragment := gitsign.FragmentPath(p, profile)
		if _, err := os.Stat(fragment); err != nil {
			return "", fmt.Errorf("profile %s's git identity is not applied yet: apply it first", profile)
		}
		return fragment, nil
	}
	return Change{
		Command: "ssh git sign use", Args: args, NoSnapshot: true,
		Subjects: []history.Subject{{Kind: history.KindProfile, Name: profile}},
		Effects: func(p paths.Paths) ([]string, error) {
			fragment, err := check(p)
			if err != nil {
				return nil, err
			}
			var out []string
			for _, d := range dirs {
				res, err := gitsign.Use(d, fragment, gitsign.Dir(p), true)
				switch {
				case err != nil:
					return nil, fmt.Errorf("%s: %w", d, err)
				case !res.Added && len(res.Replaced) == 0:
					out = append(out, d+" already commits as "+profile)
				case len(res.Replaced) > 0:
					out = append(out, fmt.Sprintf("%s will commit as %s (was %s): include.path in its own .git/config", d, profile, strings.Join(res.Replaced, ", ")))
				default:
					out = append(out, d+" will commit as "+profile+": include.path in its own .git/config")
				}
			}
			return out, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			fragment, err := check(p)
			if err != nil {
				return err
			}
			for _, d := range dirs {
				if _, err := gitsign.Use(d, fragment, gitsign.Dir(p), false); err != nil {
					return fmt.Errorf("%s: %w", d, err)
				}
				_, _ = fmt.Fprintf(out, "%s commits as %s\n", d, profile)
			}
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// gitSignPlan renders the git files from the manifest under p, edited by edit
// first when it is given (in memory: the manifest is not written here).
func gitSignPlan(p paths.Paths, edit func(*manifest.Manifest) error) (gitsign.Plan, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return gitsign.Plan{}, err
	}
	if edit != nil {
		if err := edit(m); err != nil {
			return gitsign.Plan{}, err
		}
		if err := m.Validate(); err != nil {
			return gitsign.Plan{}, err
		}
	}
	return gitsign.Render(p, m, nil)
}

// gitSignEffects lists every git file an apply writes or removes, with the
// lines that change, so the confirmation shows (and binds to) all of it.
func gitSignEffects(pl gitsign.Plan) []string {
	var out []string
	for _, prob := range pl.Problems {
		out = append(out, "warning: "+prob)
	}
	for _, w := range pl.Pending() {
		verb := "changes"
		if w.Old == "" {
			verb = "creates"
		}
		what := w.Path
		if w.Path == pl.Global {
			what += " (only its authama block; a copy is kept first)"
		}
		out = append(out, verb+" "+what)
		for _, d := range history.Diff(w.Old, w.New, 0) {
			if d.Op == '+' || d.Op == '-' {
				out = append(out, "  "+string(d.Op)+" "+d.Text)
			}
		}
	}
	for _, r := range pl.Removes {
		out = append(out, "removes "+r+" (its profile has no git identity now)")
	}
	if len(pl.Pending())+len(pl.Removes) == 0 {
		out = append(out, "the git files already match: nothing else to write")
	}
	return out
}

func applyGitSign(p paths.Paths, pl gitsign.Plan, out io.Writer) error {
	res, err := gitsign.Apply(p, pl, time.Now())
	if err != nil {
		return err
	}
	for _, w := range res.Written {
		_, _ = fmt.Fprintf(out, "wrote %s\n", filepath.Clean(w))
	}
	for _, r := range res.Removed {
		_, _ = fmt.Fprintf(out, "removed %s\n", r)
	}
	if res.Backup != "" {
		_, _ = fmt.Fprintf(out, "kept the global git config as it was in %s\n", res.Backup)
	}
	return nil
}
