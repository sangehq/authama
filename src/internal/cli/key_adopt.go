package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/reconciler"
)

// newKeyAdoptCmd is `ssh key adopt`: manage a key where it already is.
func newKeyAdoptCmd() *cobra.Command {
	var host string
	var yes bool
	cmd := &cobra.Command{
		Use:   "adopt <profile/key> <path>",
		Short: "Manage a key where it already is in ~/.ssh, without moving or renaming it",
		Long: "Declares an existing private key - ~/.ssh/id_ed25519, ~/.ssh/work/deploy_key -\n" +
			"as a profile's key, and records it. authama then renders, deploys, rotates and\n" +
			"audits it like any other, at its own path: it never moves, renames or deletes\n" +
			"the file. Rotation archives the outgoing pair in the profile's old/ and writes\n" +
			"the successor to the same path, so other tools keep finding it.\n\n" +
			"<path> is relative to ~/.ssh (a leading ~/.ssh/ is accepted). --host wires it\n" +
			"to a host in the profile.",
		Example: "  " + brand.Command + " ssh key adopt personal/github ~/.ssh/id_ed25519 --host github-personal\n" +
			"  " + brand.Command + " ssh key adopt work/deploy work/deploy_key",
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			profile, name, ok := strings.Cut(args[0], "/")
			if !ok || profile == "" || name == "" {
				return fmt.Errorf("name the key as <profile>/<key>, e.g. personal/github")
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(args[1], "~/.ssh/"), p.SSHDir+"/")
			if err := confirmChange(c, fmt.Sprintf("%s/%s will be ~/.ssh/%s, where it is; the file is never moved.", profile, name, rel), yes); err != nil {
				return err
			}
			snapshotBeforeMutation(p)
			if err := editor.AdoptKey(editor.New(p), profile, name, rel, host); err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			inv, err := inventory.Load(p.Inventory())
			if err != nil {
				return err
			}
			recorded, err := reconciler.New(p, m, inv, platform.EmitUseKeychain()).RecordExisting(manifest.KeyRef{Profile: profile, KeyName: name})
			if err != nil {
				return err
			}
			say(c, "✓ %s/%s is ~/.ssh/%s%s", profile, name, rel, map[bool]string{true: " (recorded)", false: ""}[recorded])
			if host != "" {
				say(c, "  next: `%s ssh config render` points %s at it", brand.Command, host)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "wire the key to this host in the profile")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
