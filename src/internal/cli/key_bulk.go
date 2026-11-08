package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/lifecycle"
)

// manyKeys is the keys a bulk revoke or delete acts on: named keys,
// profiles or host aliases, a category, or every key - the same selection
// `ssh rotate` takes.
func manyKeys(c *cobra.Command, sel keysvc.Selection) ([]keysvc.Row, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, err
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		return nil, err
	}
	return keysvc.New(m, inv, p.SSHDir).Select(sel)
}

// oneKey reports whether a revoke or delete names exactly one key, which
// keeps the single-key behaviour and wording.
func oneKey(c *cobra.Command, args []string, all bool, category string) bool {
	return len(args) == 1 && !all && category == "" && namesAKey(c, args[0])
}

func needsSelection(args []string, all bool, category string) error {
	if len(args) == 0 && !all && category == "" {
		return errors.New("name keys, profiles or host aliases, or pass --category <name> or --all")
	}
	return nil
}

// revokeMany revokes each selected key that was minted; one that fails is
// named and the rest go on.
func revokeMany(c *cobra.Command, sel keysvc.Selection, reason string, yes bool) error {
	rows, err := manyKeys(c, sel)
	if err != nil {
		return err
	}
	var refs []manifest.KeyRef
	var names []string
	for _, r := range rows {
		if r.Recorded && r.Fingerprint != "" {
			refs, names = append(refs, r.Ref), append(names, r.Ref.String())
		}
	}
	if len(refs) == 0 {
		return errors.New("none of those keys has been minted, so there is nothing to revoke")
	}
	if err := confirmOrAbort(c, fmt.Sprintf("Revoke %d key(s) from every target they are deployed to? They are never deployed again: %s",
		len(refs), strings.Join(names, ", ")), yes); err != nil {
		return err
	}
	p, err := resolvePaths(c)
	if err != nil {
		return err
	}
	lockForMutation(p)
	clean := true
	for _, ref := range refs {
		res, err := editor.New(p).RevokeKey(ref, reason)
		if err != nil {
			clean = false
			say(c, "%s: NOT revoked: %v", ref, err)
			continue
		}
		say(c, "revoked %s (%s)", res.Key, res.Fingerprint)
		if len(res.Removed) > 0 {
			say(c, "  removed from: %s", strings.Join(res.Removed, ", "))
		}
		if len(res.NotRemoved) > 0 {
			clean = false
			say(c, "  NOT removed from (remove it there by hand): %s", strings.Join(res.NotRemoved, ", "))
		}
	}
	say(c, "next: `%s checklist --open` lists what is left for each", brand.Command)
	if !clean {
		return errNotClean
	}
	return nil
}

// deleteMany deletes each selected key no host uses; a key a host uses is
// named and left alone, as the single delete refuses it.
func deleteMany(c *cobra.Command, sel keysvc.Selection, opt lifecycle.Options, yes, noKeyBackup bool) error {
	rows, err := manyKeys(c, sel)
	if err != nil {
		return err
	}
	var refs []manifest.KeyRef
	var names, wired []string
	for _, r := range rows {
		if r.Wired() {
			wired = append(wired, r.Ref.String())
			continue
		}
		refs, names = append(refs, r.Ref), append(names, r.Ref.String())
	}
	if len(refs) == 0 {
		return fmt.Errorf("every one of those keys is used by a host; delete or re-point the hosts first: %s", strings.Join(wired, ", "))
	}
	what := fmt.Sprintf("Delete %d key(s)? %s", len(refs), strings.Join(names, ", "))
	if opt.Purge {
		what = fmt.Sprintf("Delete %d key(s) AND their files on disk? %s", len(refs), strings.Join(names, ", "))
	}
	if err := confirmOrAbort(c, what, yes); err != nil {
		return err
	}
	p, err := resolvePaths(c)
	if err != nil {
		return err
	}
	lockForMutation(p)
	out := c.OutOrStdout()
	if opt.Purge {
		if err := backupKeysBeforeDestroying(p, out, noKeyBackup); err != nil {
			return err
		}
	}
	snapshotBeforeMutation(p)
	clean := len(wired) == 0
	for _, ref := range refs {
		res, err := lifecycle.New(p, platform.EmitUseKeychain()).DeleteKey(ref, opt)
		if err != nil {
			clean = false
			say(c, "%s: NOT deleted: %v", ref, err)
			continue
		}
		_, _ = fmt.Fprintln(out, res.Format())
	}
	if len(wired) > 0 {
		say(c, "left alone, a host uses them: %s", strings.Join(wired, ", "))
	}
	if !clean {
		return errNotClean
	}
	return nil
}
