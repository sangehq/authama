package tokensvc

import (
	"fmt"
	"io"

	"github.com/simtabi/authama/src/v3/internal/core/tokens"
)

// DockerModeFunc settles the credential-store mode for docker's file at dest.
// The CLI asks a person; the web and the TUI's callers pass a fixed answer.
type DockerModeFunc func(dest string) (string, error)

// FixedDockerMode always answers m, refusing when m is empty.
func FixedDockerMode(m string) DockerModeFunc {
	return func(string) (string, error) {
		if m == "" {
			return DockerRefuse, nil
		}
		return m, nil
	}
}

// WriteAll writes each target's entries into its file, saying where on out and
// warning on errw. Every front end writes through here, so `token set`,
// `token apply`, the TUI and the web report a write the same way.
func (e Env) WriteAll(set *tokens.Set, targets []string, project string, mode DockerModeFunc, out, errw io.Writer) error {
	for _, t := range targets {
		dest := e.TargetPath(t, project)
		if t == tokens.Docker {
			m, err := mode(dest)
			if err != nil {
				return err
			}
			res, err := e.SyncDocker(set, dest, m)
			if err != nil {
				return err
			}
			for _, w := range res.Warnings {
				_, _ = fmt.Fprintf(errw, "  %s\n", w)
			}
			for _, h := range res.LoggedIn {
				_, _ = fmt.Fprintf(out, "✓ stored docker: %s with docker login\n", h)
			}
			if len(res.Written) > 0 {
				_, _ = fmt.Fprintf(out, "✓ wrote docker → %s\n", dest)
			}
			continue
		}
		warns, err := e.WriteTarget(set, t, dest)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "✓ wrote %s → %s\n", t, dest)
		for _, w := range warns {
			_, _ = fmt.Fprintf(errw, "  %s\n", w)
		}
	}
	return nil
}

// ResolveConfig finds the token config the way `token apply` does - explicit,
// then $SSH_MANAGER_TOKENS_CONFIG, ./authama-tokens.json, tokens.json in the
// config home, the legacy locations - and falls back to the built-in default.
// note says which of the last two it used, for the caller to show.
func (e Env) ResolveConfig(explicit string) (cfg *tokens.Config, src ConfigSource, err error) {
	if src, err = e.FindConfig(explicit); err != nil {
		return nil, src, err
	}
	if src.Path == "" {
		return Builtin(), src, nil
	}
	cfg, err = LoadConfig(src.Path)
	return cfg, src, err
}
