package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/agent"
)

// newLoadCmd is the native load verb: add a profile's keys to the ssh-agent
// (macOS keychain).
func newLoadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "load <profile>",
		Short: "Add a profile's keys to the agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			profile := args[0]
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			if _, ok := m.Profiles[profile]; !ok {
				return fmt.Errorf("unknown profile: %q", profile)
			}
			a := agent.New(platform.EmitUseKeychain())
			r, err := agent.LoadReport(m, p.SSHDir, profile, a.Add)
			if err != nil {
				return err
			}
			for _, k := range r.Missing {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "skipped %s: its private key is not on disk (`ssh reconcile` mints it)\n", k)
			}
			names := "(none)"
			if len(r.Added) > 0 {
				names = strings.Join(r.Added, ", ")
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "loaded %d key(s) into the agent: %s\n", len(r.Added), names)
			if len(r.Failed) == 0 {
				return nil
			}
			// A partial load still loaded something the person can use, so it
			// warns; a load that added nothing because ssh-add failed is an
			// error, not a quiet zero (audit B-03).
			msg := fmt.Sprintf("ssh-add did not add %s (is ssh-add installed and an agent running? SSH_AUTH_SOCK names it)",
				strings.Join(r.Failed, ", "))
			if len(r.Added) == 0 {
				return errors.New(msg)
			}
			_, _ = fmt.Fprintln(c.ErrOrStderr(), "WARN: "+msg)
			return nil
		},
	}
}
