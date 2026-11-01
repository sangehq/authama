package change

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/services/tokensvc"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/paths"
)

// The token changes write credential files outside the home - ~/.npmrc,
// Composer's auth.json, docker's config.json, the git credential store,
// .netrc - so, like a Docker context, they act in OnApply and say what they
// will do in Effects. Effects never holds a secret: it names files, tools and
// hosts, which is what the confirmation binds to.

// tokenEnv reads the environment the server runs in, as the CLI does.
func tokenEnv(p paths.Paths) tokensvc.Env { return tokensvc.NewEnv(p, nil, nil, "") }

// TokenForget is `authama token forget <target> <host>`.
func TokenForget(target, host string) Change {
	return Change{
		Command: "token forget", Args: []string{target, host}, NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			if !tokens.IsTarget(target) {
				return nil, fmt.Errorf("unknown target %q", target)
			}
			env := tokenEnv(p)
			h := tokens.CanonicalHost(target, host)
			it, ok := env.Find(target, h, "")
			if !ok {
				return nil, fmt.Errorf("nothing to remove — %s: %s not found", target, h)
			}
			return []string{fmt.Sprintf("removes the %s credential for %s from %s", target, h, it.File)}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			env := tokenEnv(p)
			h := tokens.CanonicalHost(target, host)
			removed, err := env.Forget(target, h, "")
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("nothing removed — %s: %s not found", target, h)
			}
			_, _ = fmt.Fprintf(out, "✓ removed %s: %s\n", target, h)
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// TokenSync is `authama token sync` with no terminal to ask: the token config
// is resolved from the server's environment and every tool's file is merged.
// A registry docker keeps in a credential store is handled as the config's
// _settings.docker_creds_store says, and refused when it says nothing.
func TokenSync() Change {
	plan := func(p paths.Paths) (tokensvc.Env, *tokens.Set, *tokens.Config, error) {
		env := tokenEnv(p)
		cfg, _, err := env.ResolveConfig("")
		if err != nil {
			return env, nil, nil, err
		}
		set := cfg.Resolve(env.Lookup)
		if set.Len("") == 0 {
			return env, nil, nil, errors.New("no credentials resolved — set the env vars named in the token config " +
				"where the server runs, or add one with `authama token store`")
		}
		return env, set, cfg, nil
	}
	mode := func(cfg *tokens.Config) (string, error) {
		m := cfg.Setting("docker_creds_store")
		if !tokensvc.ValidDockerMode(m) {
			return "", fmt.Errorf("_settings.docker_creds_store must be one of %s, not %q",
				strings.Join(tokensvc.DockerModes(), ", "), m)
		}
		if m == "" {
			m = tokensvc.DockerRefuse
		}
		return m, nil
	}
	return Change{
		Command: "token sync", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			env, set, cfg, err := plan(p)
			if err != nil {
				return nil, err
			}
			m, err := mode(cfg)
			if err != nil {
				return nil, err
			}
			var lines []string
			for _, t := range set.Targets() {
				var hosts []string
				for _, c := range set.Entries(t) {
					hosts = append(hosts, c.Host)
				}
				sort.Strings(hosts)
				line := fmt.Sprintf("merges %s credentials for %s into %s", t, strings.Join(hosts, ", "), env.TargetPath(t, ""))
				if t == tokens.Docker {
					line += " (registries docker keeps in a credential store: " + m + ")"
				}
				lines = append(lines, line)
			}
			lines = append(lines, set.Warnings...)
			return lines, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			env, set, cfg, err := plan(p)
			if err != nil {
				return err
			}
			m, err := mode(cfg)
			if err != nil {
				return err
			}
			return env.WriteAll(set, set.Targets(), "", tokensvc.FixedDockerMode(m), out, out)
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}
