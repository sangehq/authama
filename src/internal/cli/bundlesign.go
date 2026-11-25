package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/bundlesig"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// signFlags are --sign-key, on the commands that write a bundle.
func signFlag(cmd *cobra.Command, key *string) {
	cmd.Flags().StringVar(key, "sign-key", "", "sign the file with this SSH private key, writing <file>.sig (ssh-keygen -Y sign)")
}

// verifyFlags are --require-signature and --signers, on the commands that
// read one.
func verifyFlags(cmd *cobra.Command, require *bool, signers *string) {
	cmd.Flags().BoolVar(require, "require-signature", false, "refuse the file unless <file>.sig is a good signature by a trusted signer")
	cmd.Flags().StringVar(signers, "signers", "", "trusted signers, allowed_signers lines (else bundle_signers in the config home)")
}

// signWritten signs path with key and trusts the key's own public half in
// this home's bundle_signers, so the bundle restores here with
// --require-signature; it prints the line another machine needs.
func signWritten(c *cobra.Command, p paths.Paths, path, key string) error {
	key = expandTilde(key)
	sig, err := bundlesig.Sign(path, key)
	if err != nil {
		return err
	}
	w := c.OutOrStdout()
	_, _ = fmt.Fprintf(w, "signed: %s\n", sig)
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		_, _ = fmt.Fprintf(w, "  no %s.pub to trust here; add the key's line to %s yourself\n", key, bundlesig.SignersPath(p))
		return nil
	}
	principal := signerName(string(pub))
	added, err := bundlesig.Trust(bundlesig.SignersPath(p), principal, string(pub))
	if err != nil {
		return err
	}
	if added {
		_, _ = fmt.Fprintf(w, "  trusted this key in %s\n", bundlesig.SignersPath(p))
	}
	_, _ = fmt.Fprintf(w, "  to trust it on another machine, add this line to its bundle_signers:\n  %s\n",
		bundlesig.SignerLine(principal, string(pub)))
	return nil
}

// signerName is the principal a key is trusted as: its comment, else
// "<brand>-bundle".
func signerName(pub string) string {
	if f := strings.Fields(pub); len(f) > 2 {
		return strings.Join(f[2:], "_")
	}
	return brand.Command + "-bundle"
}

// checkSignature checks path's signature, if any, before it is read,
// printing what it found when there is a signature or one is required. With
// warnUnproven it also warns when nothing shows who made the file (unsigned,
// or signed by a key no bundle_signers line trusts): a bundle or full export
// carries keys and settings, and the owner should know before restoring it.
func checkSignature(c *cobra.Command, p paths.Paths, path, signers string, require, warnUnproven bool) (bundlesig.Result, error) {
	if signers == "" {
		signers = bundlesig.SignersPath(p)
	} else {
		signers = expandTilde(signers)
	}
	res, err := bundlesig.Check(path, bundlesig.Options{Signers: signers, Require: require})
	if err != nil {
		return res, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if res.Status != bundlesig.Unsigned {
		_, _ = fmt.Fprintln(c.ErrOrStderr(), res.String())
	}
	if w := res.Warning(path); warnUnproven && w != "" {
		_, _ = fmt.Fprintln(c.ErrOrStderr(), w)
	}
	return res, nil
}

// expandTilde reads a leading ~/ as the home directory, for a path given as
// --flag=~/x, which the shell does not expand.
func expandTilde(v string) string {
	if rest, ok := strings.CutPrefix(v, "~/"); ok {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, rest)
		}
	}
	return v
}
