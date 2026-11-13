package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/bundler"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/paths"
)

func newBundleCmd() *cobra.Command {
	var recipient, output string
	var keep int
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Write an encrypted backup of keys + state (read back with restore)",
		Long: "Encrypted backup of keys + state.\n\n" +
			"Old bundles are kept, not pruned: they are encrypted, and silently deleting the\n" +
			"only copy of a key would be worse than letting them accumulate. Pass --keep to\n" +
			"trim the destination once the new bundle is written.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			// The bundle has to be of one consistent state, not of a manifest
			// and inventory another command is halfway through writing.
			lockForMutation(p)
			recip := recipient
			if recip == "" {
				recip = envnames.Get(nil, envnames.AgeRecipient)
			}
			dest := output
			if dest == "" {
				dest = p.DistDir()
			}
			stamp := time.Now().Format("20060102-150405")
			res, err := bundler.New(p.SSHDir, p.ConfigDir, bundler.AgeCipher{}).WithKeyRoot(manifest.KeyRootAt(p.Manifest())).Bundle(recip, dest, stamp)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			_, _ = fmt.Fprintln(out, res.Format())
			if keep > 0 {
				for _, pruned := range pruneBundles(dest, keep) {
					_, _ = fmt.Fprintf(out, "pruned %s\n", pruned)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&recipient, "recipient", "r", "", "age recipient (else $AUTHAMA_AGE_RECIPIENT)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "destination dir (else config-dir/dist)")
	cmd.Flags().IntVar(&keep, "keep", 0, "after bundling, keep only the newest N bundles (0 keeps all)")
	return cmd
}

// backupKeysBeforeDestroying is bundler.BackupBeforeDestroying, which the web
// UI's purge shares.
func backupKeysBeforeDestroying(p paths.Paths, out io.Writer, skip bool) error {
	return bundler.BackupBeforeDestroying(p, out, skip)
}

// pruneBundles is bundler.Prune.
func pruneBundles(dir string, keep int) []string { return bundler.Prune(dir, keep) }

func newRestoreCmd() *cobra.Command {
	var identity string
	var yes bool
	cmd := &cobra.Command{
		Use:   "restore <bundle.age>",
		Short: "Restore keys + state from an encrypted bundle (see bundle)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := confirmOrAbort(c, "Restore ~/.ssh from this bundle? (current tree is snapshotted first)", yes); err != nil {
				return err
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			ident := identity
			if ident == "" {
				if env := envnames.Get(nil, envnames.AgeIdentityFile); env != "" {
					ident = env
				}
			}
			snapshotBeforeMutation(p)
			ks := keystore.New()
			res, err := bundler.New(p.SSHDir, p.ConfigDir, bundler.AgeCipher{}).WithKeyRoot(manifest.KeyRootAt(p.Manifest())).Restore(args[0], ident, ks.Fingerprint)
			if err != nil {
				return err
			}
			finishErr := finishRestore(p)
			_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
			if finishErr != nil {
				return fmt.Errorf("keys restored, but ~/.ssh is not usable yet: %w", finishErr)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&identity, "identity", "i", "", "age identity file (else $AUTHAMA_AGE_IDENTITY_FILE)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// finishRestore is bundler.FinishRestore, which the web UI's restore shares.
func finishRestore(p paths.Paths) error { return bundler.FinishRestore(p) }
