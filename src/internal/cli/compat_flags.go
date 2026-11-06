package cli

import "github.com/spf13/cobra"

// addNoPassphraseCompat keeps v1's --no-passphrase working on the verbs that
// mint keys. v1 had a tri-state --passphrase/--no-passphrase that prompted at a
// terminal when neither was given; Go never prompts unless --passphrase is set,
// so --no-passphrase now spells the default. It was dropped silently in the
// 2.0.0 rewrite, which broke scripts that passed it; it is back, hidden, as a
// no-op that cannot be combined with --passphrase.
func addNoPassphraseCompat(cmd *cobra.Command) {
	cmd.Flags().Bool("no-passphrase", false, "v1 spelling of the default: mint keys without a passphrase")
	_ = cmd.Flags().MarkHidden("no-passphrase")
	cmd.MarkFlagsMutuallyExclusive("passphrase", "no-passphrase")
}
