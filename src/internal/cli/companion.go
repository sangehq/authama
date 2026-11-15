package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/version"
)

// companionContractVersions are the versions of sange's companion contract
// this build answers. A version is added when authama implements it, and is
// removed only by a release that says so in the CHANGELOG.
var companionContractVersions = []string{"1"}

// companionCapabilities names what a companion can ask this build for. Each is
// a command or surface that exists in it, whether or not a setting has it on:
// the API, webhooks and MCP are off by default and say so when called.
var companionCapabilities = []string{
	"git.status.json",    // ssh git status --json
	"git.use.json",       // ssh git use --json [--dry-run]
	"git.resolve.json",   // ssh git resolve <owner>/<repo> --json
	"api.read",           // /api/v1/ on serve, api.enabled
	"events.cloudevents", // the event feed, CloudEvents 1.0
	"webhooks.standard",  // Standard Webhooks v1 signatures, webhooks.enabled
	"mcp.stdio",          // authama mcp, mcp.enabled
}

// companionInfoDoc is `companion info --json`, the handshake sange's companion
// contract (v1) defines; the names are the contract's. Fields are only ever
// added.
type companionInfoDoc struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	ContractVersions []string `json:"contractVersions"`
	Capabilities     []string `json:"capabilities"`
	ConfigHome       string   `json:"configHome"`
	StateHome        string   `json:"stateHome"`
}

func newCompanionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "companion",
		Short: "Answer a companion tool such as sange: what this build can do and where it keeps things",
	}
	var asJSON bool
	info := &cobra.Command{
		Use:   "info",
		Short: "Print the name, version, contract versions, capabilities and homes; changes nothing",
		Long: "The handshake of sange's companion contract. With --json, prints {\"name\", \"version\",\n" +
			"\"contractVersions\", \"capabilities\", \"configHome\", \"stateHome\"}. A capability is listed when\n" +
			"this build has it, whether or not a setting turns it on. Fields are only ever added.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			doc := companionInfoDoc{
				Name: brand.Command, Version: version.Version,
				ContractVersions: companionContractVersions, Capabilities: companionCapabilities,
				ConfigHome: p.ConfigDir, StateHome: p.State(),
			}
			out := c.OutOrStdout()
			if asJSON {
				return writeJSONTo(out, doc)
			}
			_, _ = fmt.Fprintf(out, "%s %s\ncontract:      %s\ncapabilities:  %s\nconfig home:   %s\nstate home:    %s\n",
				doc.Name, doc.Version, strings.Join(doc.ContractVersions, ", "), strings.Join(doc.Capabilities, ", "),
				doc.ConfigHome, doc.StateHome)
			return nil
		},
	}
	info.Flags().BoolVar(&asJSON, "json", false, "machine-readable output (scripting); fields are only ever added")
	cmd.AddCommand(info)
	return cmd
}
