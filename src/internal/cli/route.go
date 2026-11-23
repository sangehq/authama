package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/util/envnames"
)

// withDirectRoute gives cmd --direct: for this run, deploy, verify and
// remove connect to the manifest's hostname, user and port, instead of
// the host's alias through the managed config (ssh.deploy_route direct).
func withDirectRoute(cmd *cobra.Command) *cobra.Command {
	var direct bool
	cmd.Flags().BoolVar(&direct, "direct", false,
		"connect to the manifest's hostname, user and port, not the host's alias through the managed config "+
			"(its ProxyJump, ProxyCommand, cloudflared or VM route); the same as ssh.deploy_route direct")
	run := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if direct {
			prev, had := os.LookupEnv(envnames.DeployRoute)
			_ = os.Setenv(envnames.DeployRoute, "direct")
			defer func() {
				if had {
					_ = os.Setenv(envnames.DeployRoute, prev)
				} else {
					_ = os.Unsetenv(envnames.DeployRoute)
				}
			}()
		}
		return run(c, args)
	}
	return cmd
}
