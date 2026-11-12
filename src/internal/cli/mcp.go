package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/services/mcp"
	"github.com/simtabi/authama/src/internal/util/envnames"
	"github.com/simtabi/authama/src/internal/version"
	"github.com/simtabi/authama/src/internal/web"
)

// errMCPOff is `authama mcp` with mcp.enabled off, which is the default.
var errMCPOff = errors.New("the MCP server is off: turn it on with `authama settings set mcp.enabled true`")

// newMCPCmd is `authama mcp`: an MCP server over stdio, for an AI client that
// starts it. Its tools read only.
func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the Model Context Protocol over stdio, for an AI client",
		Long: "An AI client (Claude Code, Claude Desktop, an IDE) starts `authama mcp` and talks to it over stdio.\n" +
			"Its tools read only: profiles, keys, hosts, which auth tokens are set (never a secret), the history,\n" +
			"the event feed and doctor. It opens no port, so only the client that started it can reach it.\n" +
			"It is off by default: `authama settings set mcp.enabled true` first.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if !settingOn(envnames.MCP, false) {
				return errMCPOff
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			srv := mcp.Server{Home: p, Reads: web.AppEngine{P: p}, Version: version.Version}
			return srv.Serve(c.Context(), c.InOrStdin(), c.OutOrStdout())
		},
	}
}
