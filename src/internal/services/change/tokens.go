package change

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/tokens"
	"github.com/sangehq/authama/src/internal/services/tokensvc"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// The token changes write credential files outside the home - ~/.npmrc,
// Composer's auth.json, docker's config.json, the git credential store,
// .netrc - so, like a Docker context, they act in OnApply and say what they
// will do in Effects. Effects never holds a secret: it names files, tools and
// hosts, which is what the confirmation binds to.

// tokenEnv reads the environment the server runs in, as the CLI does.
func tokenEnv(p paths.Paths) tokensvc.Env { return tokensvc.NewEnv(p, nil, nil, "") }

// TokenDelete is `authama token delete <target> <host>`.
func TokenDelete(target, host string) Change {
	return Change{
		Command: "token delete", Args: []string{target, host}, NoSnapshot: true,
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

// TokenApply is `authama token apply` with no terminal to ask: the token config
// is resolved from the server's environment and every tool's file is merged.
// A registry docker keeps in a credential store is handled as the config's
// _settings.docker_creds_store says, and refused when it says nothing.
func TokenApply() Change {
	plan := func(p paths.Paths) (tokensvc.Env, *tokens.Set, *tokens.Config, error) {
		env := tokenEnv(p)
		cfg, _, err := env.ResolveConfig("")
		if err != nil {
			return env, nil, nil, err
		}
		set := cfg.Resolve(env.Lookup)
		if set.Len("") == 0 {
			return env, nil, nil, errors.New("no credentials resolved — set the env vars named in the token config " +
				"where the server runs, or add one with `authama token set`")
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
		Command: "token apply", NoSnapshot: true,
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

// TokenSet is `authama token set`: add one credential to its tool's file, or
// replace the one already there for that host. The secret is not part of the
// change - Effects and the command line never hold it - and comes from
// secret only when the change is made. mode settles a registry Docker keeps in
// a credential store as --creds-store-mode does; empty reads
// $AUTHAMA_DOCKER_CREDS_STORE, and refuses when that is unset too, since a page
// cannot ask.
func TokenSet(spec tokensvc.SetSpec, mode string, secret func() (string, error)) Change {
	var args []string
	if spec.Service != "" {
		args = append(args, "--service", spec.Service)
	} else {
		args = append(args, "--target", spec.Target)
	}
	args = append(args, "--server", spec.Server)
	if spec.Username != "" {
		args = append(args, "--username", spec.Username)
	}
	if spec.Scope != "" {
		args = append(args, "--scope", spec.Scope)
	}
	if mode != "" {
		args = append(args, "--creds-store-mode", mode)
	}
	dockerMode := func() (string, error) {
		m := mode
		if m == "" {
			m = strings.TrimSpace(envnames.Get(nil, envnames.DockerCredsStore))
		}
		if !tokensvc.ValidDockerMode(m) {
			return "", fmt.Errorf("the Docker credential-store mode must be one of %s, not %q",
				strings.Join(tokensvc.DockerModes(), ", "), m)
		}
		if m == "" {
			m = tokensvc.DockerRefuse
		}
		return m, nil
	}
	return Change{
		Command: "token set", Args: args, NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			plan, err := spec.Check()
			if err != nil {
				return nil, err
			}
			if _, err := dockerMode(); err != nil {
				return nil, err
			}
			env := tokenEnv(p)
			host := tokens.CanonicalHost(plan.Target, strings.TrimSpace(spec.Server))
			verb := "adds"
			if _, ok := env.Find(plan.Target, host, ""); ok {
				verb = "replaces"
			}
			what := "token"
			if !plan.Token {
				what = "password for " + spec.Username
			}
			return []string{fmt.Sprintf("%s the %s %s for %s in %s (the secret is not shown)",
				verb, plan.Target, what, host, env.TargetPath(plan.Target, ""))}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			plan, err := spec.Check()
			if err != nil {
				return err
			}
			m, err := dockerMode()
			if err != nil {
				return err
			}
			s, err := secret()
			if err != nil {
				return err
			}
			set, err := plan.Credential(s)
			if err != nil {
				return err
			}
			return tokenEnv(p).WriteAll(set, []string{plan.Target}, "", tokensvc.FixedDockerMode(m), out, out)
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// TokenInit is `authama token init [--target …]` with no FILE: a starter token
// config at the config home's tokens.json, never over one that is there. It
// names environment variables, never secrets.
func TokenInit(targets []string) Change {
	var args []string
	for _, t := range targets {
		args = append(args, "--target", t)
	}
	return Change{
		Command: "token init", Args: args, NoSnapshot: true,
		Run: func(p paths.Paths, out io.Writer) error {
			for _, t := range targets {
				if !tokens.IsTarget(t) {
					return fmt.Errorf("unknown target %q (choose from %s)", t, strings.Join(tokens.Targets(), ", "))
				}
			}
			msg, err := tokenEnv(p).Scaffold("", targets)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s\nEdit it, then run `%s token apply`.\n", msg, brand.Command)
			return nil
		},
	}
}

// TokenImport is `authama token import` with no FILE: copy auth-token-manager's
// config, found where that tool looked, to the config home's tokens.json. The
// old file is left in place.
func TokenImport() Change {
	return Change{
		Command: "token import", NoSnapshot: true,
		Run: func(p paths.Paths, out io.Writer) error {
			src, to, err := tokenEnv(p).Import("", false)
			if errors.Is(err, tokensvc.ErrNoLegacyConfig) {
				_, _ = fmt.Fprintln(out, "No auth-token-manager config found; nothing to import.")
				return nil
			}
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "✓ copied %s → %s\nThe old file is left in place; delete it once you are happy with the new one.\n", src, to)
			return nil
		},
	}
}

// TokenConfigEdit replaces the config home's tokens.json with text, the web
// UI's editor: the text must parse and hold variable names only
// (tokens.CheckNamesOnly), so the editor can never be the way a secret lands
// in a file that is meant to be committed. The preview shows the file's diff.
// The CLI's equivalent is editing the file and running `token status`.
func TokenConfigEdit(text string) Change {
	return Change{
		Command: "token config edit",
		Run: func(p paths.Paths, out io.Writer) error {
			if err := tokens.CheckNamesOnly(text); err != nil {
				return err
			}
			body := strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n") + "\n"
			if err := fs.WriteTextAtomic(p.Tokens(), body, tokensvc.ConfigMode); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "✓ wrote %s\nRun `%s token apply` to merge it into each tool's file.\n", p.Tokens(), brand.Command)
			return nil
		},
	}
}
