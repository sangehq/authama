package change

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/gitremote"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// RemoteGit is the git the git-remote change runs; tests swap it.
var RemoteGit gitremote.Git = gitremote.Exec{}

// GitRemoteUse is `authama ssh git use <mode> <path>... [--profile p] --yes`:
// each checkout's origin pointed at mode. Nothing it changes is in a copy a
// preview makes (the remotes are in each checkout's .git/config), so Effects
// lists what each checkout would go from and to, worked out without touching
// it, and OnApply proves each new URL with ls-remote before it rewrites
// origin, as the command line does. A checkout that fails is left as it was
// and the rest still change; the change then reports which.
func GitRemoteUse(mode, profile string, dirs []string) Change {
	args := []string{}
	if mode != "" {
		args = append(args, mode)
	}
	args = append(args, dirs...)
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	args = append(args, "--yes")
	resolve := func(p paths.Paths) (*manifest.Manifest, string, error) {
		m, err := manifest.Load(p.Manifest())
		if err != nil {
			return nil, "", err
		}
		md := mode
		if md == "" {
			md = m.Defaults.Git.EffectiveMode()
		}
		switch {
		case !slices.Contains(manifest.GitModes, md):
			return nil, "", fmt.Errorf("mode %q is not one of %s", md, strings.Join(manifest.GitModes, ", "))
		case profile != "" && !hasProfile(m, profile):
			return nil, "", fmt.Errorf("unknown profile: %q", profile)
		case len(dirs) == 0:
			return nil, "", errors.New("name at least one checkout")
		}
		return m, md, nil
	}
	var subjects []history.Subject
	if profile != "" {
		subjects = []history.Subject{{Kind: history.KindProfile, Name: profile}}
	}
	return Change{
		Command: "ssh git use", Args: args, NoSnapshot: true, Subjects: subjects,
		Effects: func(p paths.Paths) ([]string, error) {
			m, md, err := resolve(p)
			if err != nil {
				return nil, err
			}
			var out []string
			for _, d := range dirs {
				ch, err := gitremote.Use(RemoteGit, m, d, md, profile, true)
				switch {
				case err != nil:
					out = append(out, d+": cannot change: "+err.Error())
				case ch.Skipped != "":
					out = append(out, d+": skipped: "+ch.Skipped)
				default:
					out = append(out, fmt.Sprintf("%s: origin %s -> %s, once ls-remote proves it", d, ch.From, ch.To))
				}
			}
			return out, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			m, md, err := resolve(p)
			if err != nil {
				return err
			}
			var failed []string
			for _, d := range dirs {
				ch, err := gitremote.Use(RemoteGit, m, d, md, profile, false)
				switch {
				case err != nil:
					failed = append(failed, d)
					_, _ = fmt.Fprintf(out, "%s: %v\n", d, err)
				case ch.Skipped != "":
					_, _ = fmt.Fprintf(out, "%s  skipped: %s\n", d, ch.Skipped)
				default:
					_, _ = fmt.Fprintf(out, "%s  %s -> %s\n", d, ch.From, ch.To)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("%d checkout(s) left unchanged: %s", len(failed), strings.Join(failed, ", "))
			}
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

func hasProfile(m *manifest.Manifest, name string) bool {
	_, ok := m.Profiles[name]
	return ok
}
