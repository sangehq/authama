package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
)

// newKeyRootCmd is `ssh key root`: where under ~/.ssh the profile directories
// live, and moving them.
func newKeyRootCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "root [<dir>]",
		Short: "Show or move the directory under ~/.ssh that holds the profile directories",
		Long: "With no argument, prints the key root: the directory under ~/.ssh that holds\n" +
			"the profile directories, ~/.ssh/profiles/ unless defaults.key_root names\n" +
			"another.\n\n" +
			"With <dir>, makes ~/.ssh/<dir>/ the key root and moves the whole tree there in\n" +
			"one rename: private keys, .pub files, old/ archives and category config files.\n" +
			"The inventory follows and ~/.ssh/config is rendered again, so ssh finds every\n" +
			"key at its new path. Keys adopted where they are stay put. ~/.ssh is\n" +
			"snapshotted first.\n\n" +
			"<dir> is one directory name, not hidden and not one of ssh's own files.\n" +
			"\"profiles\" returns to the default. It refuses when ~/.ssh/<dir> already\n" +
			"exists beside the current root: merge the two by hand first.",
		Example: "  " + brand.Command + " ssh key root\n" +
			"  " + brand.Command + " ssh key root keys\n" +
			"  " + brand.Command + " ssh key root profiles --yes",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			current := manifest.KeyRootAt(p.Manifest())
			if len(args) == 0 {
				say(c, "~/.ssh/%s/", current)
				return nil
			}
			target := args[0]
			if target == "" {
				target = manifest.DefaultKeyRoot
			}
			if err := manifest.CheckKeyRoot(target); err != nil {
				return err
			}
			if target == current {
				say(c, "the key root is already ~/.ssh/%s/", current)
				return nil
			}
			if err := confirmChange(c, fmt.Sprintf("~/.ssh/%s/ moves to ~/.ssh/%s/, keys and all, and ~/.ssh/config is rendered again.",
				current, target), yes); err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			res, err := editor.MoveKeyRoot(editor.New(p), target)
			if err != nil {
				return err
			}
			verb := "keys will be minted under"
			if res.Moved {
				verb = "moved the keys to"
			}
			say(c, "✓ %s %s (was %s); %d inventory record(s) updated",
				verb, filepath.ToSlash(filepath.Join("~/.ssh", res.To))+"/", "~/.ssh/"+res.From+"/", res.RecordsUpdated)
			return applyManifestEdit(c, p)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}
