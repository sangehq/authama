package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/keysvc"
)

// newKeyLifetimeCmd is `ssh key lifetime`: how long a key lives before
// rotation is due. The expiry date is counted again from when it was made.
func newKeyLifetimeCmd() *cobra.Command {
	var asDefault, clear, yes bool
	cmd := &cobra.Command{
		Use:   "lifetime <profile/key> <days|annual|semiannual|quarterly|monthly>",
		Short: "Set how long a key lives before rotation is due, and count its expiry again",
		Long: "Sets a key's rotate_after_days and counts its expiry date again from when the\n" +
			"key was created. A preset names a common policy: annual (365 days, the usual\n" +
			"floor for SSH user keys), semiannual (180), quarterly (90) or monthly (30).\n" +
			"Anything from 1 to 1825 days is accepted.\n\n" +
			"--default sets the lifetime every key without its own inherits, and takes the\n" +
			"lifetime alone. --clear drops a key's own value, so it inherits the default.",
		Example: "  " + brand.Command + " ssh key lifetime work/work_gh-ed25519 quarterly\n" +
			"  " + brand.Command + " ssh key lifetime work/work_gh-ed25519 120\n" +
			"  " + brand.Command + " ssh key lifetime --default annual\n" +
			"  " + brand.Command + " ssh key lifetime work/work_gh-ed25519 --clear",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			ed := editor.New(p)
			out := c.OutOrStdout()
			if asDefault {
				if len(args) != 1 || clear {
					return errors.New("--default takes the lifetime alone: `ssh key lifetime --default <days|preset>`")
				}
				days, err := editor.ParseLifetime(args[0])
				if err != nil {
					return err
				}
				if err := confirmChange(c, fmt.Sprintf("Every key without a lifetime of its own will live %d days.", days), yes); err != nil {
					return err
				}
				snapshotBeforeMutation(p)
				res, err := ed.SetDefaultLifetime(days)
				if err != nil {
					return err
				}
				say(c, "default lifetime: %d days (%d key(s) recounted)", days, len(res))
				for _, r := range res {
					_, _ = fmt.Fprintf(out, "  %s  expires %s\n", r.Key, emDash(r.ExpiresOn))
				}
				return nil
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			ref, err := m.ResolveKeySelector(args[0])
			if err != nil {
				return err
			}
			days := 0
			switch {
			case clear && len(args) == 1:
			case clear:
				return errors.New("--clear takes the key alone")
			case len(args) != 2:
				return errors.New("give the lifetime: days, or annual, semiannual, quarterly or monthly")
			default:
				if days, err = editor.ParseLifetime(args[1]); err != nil {
					return err
				}
			}
			what := fmt.Sprintf("%s will live %d days.", ref, days)
			if clear {
				what = fmt.Sprintf("%s will inherit the default lifetime (%d days).", ref, m.Defaults.RotateAfterDays)
			}
			if err := confirmChange(c, what, yes); err != nil {
				return err
			}
			snapshotBeforeMutation(p)
			res, err := ed.SetKeyLifetime(ref, days)
			if err != nil {
				return err
			}
			say(c, "%s: %d days, expires %s", res.Key, res.Days, emDash(res.ExpiresOn))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asDefault, "default", false, "set the lifetime every key without its own inherits")
	cmd.Flags().BoolVar(&clear, "clear", false, "drop the key's own lifetime, so it inherits the default")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// newKeyRevokeCmd is `ssh key revoke`: withdraw a key from everywhere it is
// deployed, keep it declared, and never deploy that key again.
func newKeyRevokeCmd() *cobra.Command {
	var reason, category string
	var yes, all bool
	cmd := &cobra.Command{
		Use:   "revoke [<profile/key>|profile|alias ...] [--category <name>] [--all]",
		Short: "Remove a key from every target it is deployed to, and never deploy it again",
		Long: "Asks each target the key is deployed to, through its host's provider, to remove\n" +
			"it: GitHub and GitLab through their CLIs, a cloud API, a server's\n" +
			"authorized_keys. The deployments it was removed from are dropped from its\n" +
			"record, and its fingerprint is recorded in revoked.json, so deploy and\n" +
			"rollback refuse it from then on. A target that cannot be reached, or whose\n" +
			"provider cannot remove keys, is named so you can remove it by hand.\n\n" +
			"The key's declaration, hosts and files stay. Mint its successor next:\n" +
			"`ssh keygen <profile/key> --force` now, or `ssh rotate <profile/key>` staged.\n\n" +
			"Name several keys, profiles or host aliases, a --category or --all to revoke\n" +
			"each minted key among them in turn; one that fails is named and the rest go on.",
		Example: "  " + brand.Command + " ssh key revoke work/work_gh-ed25519 --reason \"laptop lost\"\n" +
			"  " + brand.Command + " ssh key revoke work personal/personal_gh-ed25519 --yes\n" +
			"  " + brand.Command + " ssh key revoke --category clients --yes",
		Args: cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if err := needsSelection(args, all, category); err != nil {
				return err
			}
			if !oneKey(c, args, all, category) {
				return revokeMany(c, keysvc.Selection{Selectors: args, Category: category, All: all}, strings.TrimSpace(reason), yes)
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			ref, err := m.ResolveKeySelector(args[0])
			if err != nil {
				return err
			}
			if err := confirmOrAbort(c, fmt.Sprintf("Revoke %s from every target it is deployed to? It is never deployed again.", ref), yes); err != nil {
				return err
			}
			lockForMutation(p)
			res, err := editor.New(p).RevokeKey(ref, strings.TrimSpace(reason))
			if err != nil {
				return err
			}
			say(c, "revoked %s (%s)", res.Key, res.Fingerprint)
			if len(res.Removed) > 0 {
				say(c, "  removed from: %s", strings.Join(res.Removed, ", "))
			}
			if len(res.NotRemoved) > 0 {
				say(c, "  NOT removed from (remove it there by hand): %s", strings.Join(res.NotRemoved, ", "))
			}
			say(c, "next: `%s ssh keygen %s --force` mints its successor now; `%s ssh rotate %s` stages it", brand.Command, ref, brand.Command, ref)
			if len(res.NotRemoved) > 0 {
				return errNotClean
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "why, recorded with the revocation (e.g. \"laptop lost\")")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&all, "all", false, "revoke every minted key")
	cmd.Flags().StringVar(&category, "category", "", "revoke every minted key in profiles of this category")
	return cmd
}
