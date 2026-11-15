package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/importer"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// newImportCmd is the native import verb: onboard an existing ~/.ssh into the
// manifest + inventory. import REPLACES them wholesale (it does not merge), so a
// non-empty manifest is refused without --force (and both are backed up first).
func newImportCmd() *cobra.Command {
	var dryRun, force, yes bool
	cmd := &cobra.Command{
		Use:   "import [PATH]",
		Short: "Onboard an existing ssh setup - a config file or any folder of them - into the manifest",
		Long: "Reads an ssh setup into the manifest and inventory, replacing them (a non-empty\n" +
			"manifest needs --force, and both are backed up first). PATH defaults to\n" +
			"~/.ssh/config and may be a config file or a folder:\n\n" +
			"  a folder with a config file    that file, whose Includes pull in the rest\n" +
			"                                 (config.d/, profiles/<name>/config, …)\n" +
			"  a folder with ssh/config       that file, for a config home such as ~/.config\n" +
			"  any other folder               every file named config or ending .conf, up to\n" +
			"                                 four levels down, read in path order\n\n" +
			"Relative Includes resolve against the folder, as ssh resolves them against\n" +
			"~/.ssh. Importing another machine's copy of ~/.ssh, an IdentityFile under\n" +
			"~/.ssh/ is looked for in that copy, and its keys are adopted into the\n" +
			"profile directories here (~/.ssh/profiles/, or the directory\n" +
			"defaults.key_root names).",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			configPath := "~/.ssh/config"
			if len(args) > 0 {
				configPath = args[0]
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			// Asked before the lock: the question names paths, not state, so
			// nothing it shows can go stale while someone reads it.
			if !dryRun {
				if err := confirmChange(c,
					"import replaces "+p.Manifest()+" with what it reads from "+configPath+
						",\nand copies any key it finds outside the managed tree into it.", yes); err != nil {
					return err
				}
			}
			lockForMutation(p)
			imp := importer.New(p, platform.EmitUseKeychain())

			if dryRun {
				res, err := imp.Run(configPath, true)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
				return nil
			}

			// import replaces the manifest + inventory (config home, outside the
			// ~/.ssh snapshot). Refuse to clobber a non-empty manifest without --force.
			snapshotBeforeMutation(p)
			if m, err := manifest.Load(p.Manifest()); err == nil && len(m.Profiles) > 0 {
				if !force {
					return change.ErrImportWouldReplace
				}
				backupImportTargets(p)
			}
			res, err := imp.Run(configPath, false)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "parse and report without writing")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "replace an existing non-empty manifest (backed up first)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "import without confirming (implied when there is no terminal)")
	return cmd
}

// backupImportTargets copies manifest + inventory into <home>/.state/import-backup-<ts>/.
//
// Deprecated: use importer.BackupTargets, which the web's import shares. This
// forwards to it and stays until at least v1.0.0.
func backupImportTargets(p paths.Paths) { importer.BackupTargets(p) }
