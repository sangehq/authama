package cli

import (
	"fmt"
	"github.com/sangehq/authama/src/internal/services/reconciler"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/rotator"
)

func newRotateCmd() *cobra.Command {
	var allowUnverified, passphrase, yes, noKeyBackup, all, dryRun, check, force, evictOld bool
	var category string
	var failed bool
	cmd := &cobra.Command{
		Use: "rotate [<[profile/]key>|profile|alias ...] [--category <name>] [--all]",
		// recycle is what a policy calls a staged rotation: the old key is
		// retired and its successor deployed and verified before it goes.
		Aliases: []string{"recycle"},
		Short:   "Zero-downtime staged key rotation",
		Long: "Zero-downtime staged key rotation.\n\n" +
			"The key being replaced is archived to the profile's old/ dir, but only one\n" +
			"predecessor is kept. Rotating again stops while that predecessor is still\n" +
			"authorised on a host it was never removed from; --evict-old replaces it anyway.\n" +
			"An encrypted bundle is written first unless --no-key-backup is given.\n\n" +
			"Name one key, several, whole profiles or host aliases, every key in a\n" +
			"category, or --all. Each key rotates on its own: one that fails is left\n" +
			"untouched and the rest still rotate. A key no host uses is skipped, since\n" +
			"there is nothing to deploy it to.",
		RunE: func(c *cobra.Command, args []string) error {
			if failed {
				if len(args) > 0 || all || category != "" {
					return fmt.Errorf("--failed runs the keys the last rotation left; name no keys, --all or --category with it")
				}
				p, err := resolvePaths(c)
				if err != nil {
					return err
				}
				left, err := rotator.LoadFailed(p)
				if err != nil {
					return err
				}
				if len(left) == 0 {
					return fmt.Errorf("no key was left by the last rotation or dry run, so --failed has nothing to run")
				}
				return rotateMany(c, keysvc.Selection{Selectors: left}, allowUnverified, passphrase, yes, noKeyBackup, dryRun, check, force, evictOld)
			}
			if len(args) == 1 && !all && category == "" && !dryRun && !check && namesAKey(c, args[0]) {
				return rotateOne(c, args[0], allowUnverified, passphrase, yes, noKeyBackup, force, evictOld)
			}
			return rotateMany(c, keysvc.Selection{Selectors: args, Category: category, All: all},
				allowUnverified, passphrase, yes, noKeyBackup, dryRun, check, force, evictOld)
		},
	}
	cmd.Flags().BoolVar(&allowUnverified, "allow-unverified", false, "commit even if a target can't auto-verify")
	cmd.Flags().BoolVar(&passphrase, "passphrase", false, "protect the rotated-in key (prompts without echo)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	cmd.Flags().BoolVar(&noKeyBackup, "no-key-backup", false,
		"rotate without writing an encrypted backup first (the earlier predecessor is lost)")
	cmd.Flags().BoolVar(&all, "all", false, "rotate every key a host uses")
	cmd.Flags().StringVar(&category, "category", "", "rotate every key in profiles of this category")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list the keys that would rotate, and change nothing")
	cmd.Flags().BoolVar(&force, "force", false,
		"replace on this computer (as ssh keygen --force) each key that cannot be rotated in place - never deployed, or its host unreachable - to deploy afterwards")
	cmd.Flags().BoolVar(&evictOld, "evict-old", false,
		"replace the previous key in old/ even while a host it was never removed from still accepts it")
	cmd.Flags().BoolVar(&failed, "failed", false,
		"run again the keys the last rotation or dry run left (not rotated, or would fail)")
	cmd.Flags().BoolVar(&check, "check", false,
		"test each key's rotation and change nothing: every server answers and the current key logs in, or the target takes it by hand")
	addNoPassphraseCompat(cmd)
	return cmd
}

// namesAKey reports whether a single rotate argument is a key selector, the
// only thing rotateOne resolves. A profile or alias - both in the usage line,
// and both accepted by --dry-run - goes to the batch path, which resolves every
// kind of selector; sending it to rotateOne failed with "no such key".
func namesAKey(c *cobra.Command, arg string) bool {
	p, err := resolvePaths(c)
	if err != nil {
		return true // let rotateOne report the real problem
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return true
	}
	_, err = m.ResolveKeySelector(arg)
	return err == nil
}

// rotateOne is the single-key rotation, unchanged from before batches existed.
func rotateOne(c *cobra.Command, key string, allowUnverified, passphrase, yes, noKeyBackup, force, evictOld bool) error {
	if err := confirmOrAbort(c, fmt.Sprintf("Rotate %s? (the current key moves to old/, "+
		"replacing any earlier predecessor)", key), yes); err != nil {
		return err
	}
	p, err := resolvePaths(c)
	if err != nil {
		return err
	}
	lockForMutation(p)
	// Refused before the backup: a backup written for a rotation that can
	// never run is a bundle left behind for nothing.
	if err := p.RefuseOutsideSandbox("rotate"); err != nil {
		return err
	}
	if err := backupKeysBeforeDestroying(p, c.OutOrStdout(), noKeyBackup); err != nil {
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
	pw := ""
	if passphrase {
		secret, err := readPassphrase(c.InOrStdin())
		if err != nil {
			return err
		}
		pw = secret
	}
	snapshotBeforeMutation(p)
	report, err := withManualPaste(c, evicting(rotator.New(p, m, inv), evictOld)).Rotate(key, allowUnverified, pw)
	if err != nil {
		return err
	}
	if report.Committed {
		if err := inv.Save(p.Inventory()); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(c.OutOrStdout(), report.Format())
	if !report.Committed {
		if force {
			if _, still := reconciler.ReplaceLocally(p, []string{key}, platform.EmitUseKeychain(), c.OutOrStdout()); len(still) == 0 {
				return nil
			}
		}
		return errNotClean
	}
	return nil
}

// rotateMany rotates a selection of keys one after another: one confirmation,
// one backup, one snapshot, and the inventory saved after every committed key
// so an interrupted batch keeps what it finished.
func rotateMany(c *cobra.Command, sel keysvc.Selection, allowUnverified, passphrase, yes, noKeyBackup, dryRun, check, force, evictOld bool) error {
	out := c.OutOrStdout()
	p, err := resolvePaths(c)
	if err != nil {
		return err
	}
	lockForMutation(p)
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		return err
	}
	rows, err := keysvc.New(m, inv, p.SSHDir).Select(sel)
	if err != nil {
		return err
	}
	var todo []keysvc.Row
	for _, r := range rows {
		if !r.Wired() {
			_, _ = fmt.Fprintf(out, "skip     %s - no host uses it, so there is nothing to rotate it on\n", r.Ref)
			continue
		}
		todo = append(todo, r)
	}
	if len(todo) == 0 {
		_, _ = fmt.Fprintln(out, "nothing to rotate")
		return nil
	}
	names := make([]string, len(todo))
	for i, r := range todo {
		names[i] = r.Ref.String()
	}
	if dryRun {
		for _, n := range names {
			_, _ = fmt.Fprintln(out, "rotate   "+n)
		}
		_, _ = fmt.Fprintf(out, "(dry run) %d key(s) would rotate\n", len(names))
		return nil
	}
	if check {
		r := rotator.New(p, m, inv)
		var failed []string
		var reports []rotator.CheckReport
		began := time.Now()
		for i, n := range names {
			stop := startProgress(c, fmt.Sprintf("testing %d of %d: %s", i+1, len(names), n))
			rep, err := r.Check(n)
			stop()
			if err != nil {
				return err
			}
			reports = append(reports, rep)
			if !rep.OK() {
				failed = append(failed, n)
			}
		}
		sum := rotator.SummarizeCheck(reports, time.Since(began))
		sum.Command = commandLine(c)
		_, _ = fmt.Fprint(out, sum.Format())
		if err := rotator.SaveFailed(p, sum.Failed()); err != nil {
			return err
		}
		if len(failed) > 0 {
			return fmt.Errorf("%d of %d key(s) would not rotate cleanly: %s; nothing was changed", len(failed), len(names), strings.Join(failed, ", "))
		}
		_, _ = fmt.Fprintf(out, "(check) %d key(s) would rotate; nothing was changed\n", len(names))
		return nil
	}
	if err := confirmOrAbort(c, fmt.Sprintf("Rotate %d key(s): %s? (each current key moves to old/, "+
		"replacing any earlier predecessor)", len(names), strings.Join(names, ", ")), yes); err != nil {
		return err
	}
	if err := p.RefuseOutsideSandbox("rotate"); err != nil {
		return err
	}
	if err := backupKeysBeforeDestroying(p, out, noKeyBackup); err != nil {
		return err
	}
	pw := ""
	if passphrase {
		secret, err := readPassphrase(c.InOrStdin())
		if err != nil {
			return err
		}
		pw = secret
	}
	snapshotBeforeMutation(p)
	rot := withManualPaste(c, evicting(rotator.New(p, m, inv), evictOld))
	began := time.Now()
	failed, err := rot.RotateEach(names, allowUnverified, pw, out, func() error { return inv.Save(p.Inventory()) }, nil)
	if err != nil {
		return err
	}
	var replaced []string
	if force && len(failed) > 0 {
		_, _ = fmt.Fprintf(out, "forced: replacing %d key(s) the rotation could not rotate, on this computer\n", len(failed))
		replaced, failed = reconciler.ReplaceLocally(p, failed, platform.EmitUseKeychain(), out)
	}
	sum := rotator.SummarizeRun(rot.Results(), replaced, time.Since(began))
	sum.Command = commandLine(c)
	if len(names) > 1 {
		_, _ = fmt.Fprint(out, "\n"+sum.Format())
	}
	if err := rotator.SaveFailed(p, sum.Failed()); err != nil {
		return err
	}
	if len(failed) > 0 {
		return errNotClean
	}
	return nil
}

func newRollbackCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rollback <[profile/]key>",
		Short: "Undo a rotation: put a key's previous version back",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			key := args[0]
			if err := confirmOrAbort(c, fmt.Sprintf("Roll back %s to its /old/ predecessor?", key), yes); err != nil {
				return err
			}
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			lockForMutation(p)
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			inv, err := inventory.Load(p.Inventory())
			if err != nil {
				return err
			}
			snapshotBeforeMutation(p)
			report, err := rotator.New(p, m, inv).Rollback(key)
			if err != nil {
				return err
			}
			if err := inv.Save(p.Inventory()); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), report.Format())
			if !report.Committed {
				return errNotClean // incomplete: a script must not read it as done
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

// withManualPaste makes rotate wait at a web-panel target (Ploi and the like): it shows the staged
// public key and where it goes, waits until the user says it is pasted, and the rotator then proves it
// by logging in with the staged key before anything is committed. With no terminal to ask at, rotate
// keeps its old behaviour: such a target cannot verify, so it aborts unless --allow-unverified.
func withManualPaste(c *cobra.Command, r *rotator.Rotator) *rotator.Rotator {
	if !platform.StdinIsTerminal() {
		return r
	}
	return r.WithManualStep(func(alias, where, pubkey string) bool {
		out := c.OutOrStdout()
		_, _ = fmt.Fprintf(out, "\n%s: this target takes the key by hand. %s\n\n%s\n\n", alias, where, strings.TrimSpace(pubkey))
		return confirm(c, "  Pasted it? authama will now log in with the new key to prove it")
	})
}

// commandLine is the command as it was typed, for a summary's first line:
// its path, the flags given, and its arguments.
func commandLine(c *cobra.Command) string {
	parts := []string{c.CommandPath()}
	c.Flags().Visit(func(f *pflag.Flag) {
		if f.Value.Type() == "bool" {
			parts = append(parts, "--"+f.Name)
			return
		}
		parts = append(parts, "--"+f.Name, f.Value.String())
	})
	return strings.Join(append(parts, c.Flags().Args()...), " ")
}

// evicting is r, allowed to replace a predecessor in old/ that still logs in
// when evict is set (--evict-old).
func evicting(r *rotator.Rotator, evict bool) *rotator.Rotator {
	r.EvictOld = evict
	return r
}
