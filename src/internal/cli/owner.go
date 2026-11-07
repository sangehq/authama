package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/owner"
	"github.com/simtabi/authama/src/internal/util/log"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Audit events for the owner binding. None carries a PIN or the key.
const (
	eventOwnerInit      = "owner.init"
	eventOwnerForget    = "owner.forget"
	eventOwnerReseal    = "owner.reseal"
	eventOwnerPINSet    = "owner.pin.set"
	eventOwnerPINReset  = "owner.pin.reset"
	eventOwnerPINRemove = "owner.pin.remove"
	eventOwnerBiometric = "owner.biometric"
)

func newOwnerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "owner",
		Short: "Bind this home to you and this machine: a PIN, and a seal that notices outside changes",
		Long: "Binds the authama home to the OS account and machine it runs on.\n\n" +
			"`owner init` keeps a sealing key in the OS's credential store (the macOS\n" +
			"Keychain, the Secret Service on Linux, DPAPI on Windows, else an owner-only\n" +
			"file) and seals the state files with it. authama reseals after each of its own\n" +
			"operations, so a file changed by anything else, or a home copied to another\n" +
			"account or machine, is named the next time authama runs.\n\n" +
			"`owner pin set` adds a PIN or passphrase. The web UI asks for it at sign-in and\n" +
			"before every change; the command line asks before revealing or copying a secret\n" +
			"and before a destructive command. A forgotten PIN is reset with `owner pin\n" +
			"reset`, which needs no old PIN, only this account on this machine.",
	}
	cmd.AddCommand(newOwnerStatusCmd(), newOwnerInitCmd(), newOwnerVerifyCmd(), newOwnerResealCmd(),
		newOwnerForgetCmd(), newOwnerPINCmd(), newOwnerBiometricCmd())
	return cmd
}

func newOwnerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the home is bound, where its key is kept, and whether a PIN is set",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			st := owner.Verify(p)
			if !st.SetUp {
				_, _ = fmt.Fprintf(out, "not bound: run `%s owner init` to bind this home to you and this machine\n", brand.Command)
				return nil
			}
			cfg := st.Config
			if cfg == nil {
				return errors.New(st.Problem)
			}
			yesNo := map[bool]string{true: "yes", false: "no"}
			bio, why := owner.BiometricAvailable()
			_, _ = fmt.Fprintf(out, "bound since  %s\n", cfg.Created.Local().Format("2006-01-02 15:04"))
			_, _ = fmt.Fprintf(out, "key kept in  %s\n", cfg.Store)
			_, _ = fmt.Fprintf(out, "PIN          %s\n", yesNo[cfg.HasPIN()])
			_, _ = fmt.Fprintf(out, "fingerprint  %s", yesNo[cfg.Biometric])
			if !bio {
				_, _ = fmt.Fprintf(out, " (%s)", why)
			}
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintf(out, "seal         %s\n", st.Describe())
			return nil
		},
	}
}

func newOwnerInitCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bind this home to you and this machine, and seal its state files",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			where := owner.StoreFor(p).Name()
			if err := confirmChange(c, fmt.Sprintf("A sealing key will be kept in %s, and the state files in %s sealed with it.", where, p.ConfigDir), yes); err != nil {
				return err
			}
			mutation.LockForMutation(p)
			cfg, err := owner.Init(p, time.Now())
			if err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerInit, log.Field{Key: "store", Value: cfg.Store})
			say(c, "✓ bound to this account and machine; the key is in %s", cfg.Store)
			say(c, "  next: `%s owner pin set` adds a PIN", brand.Command)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newOwnerVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Check the state files against the seal, and name any changed outside authama",
		Long: "Exits 0 when the seal matches (or the home is not bound), and non-zero when a\n" +
			"file was changed, removed or added since authama last sealed it, when the seal\n" +
			"was made on another account or machine, or when it cannot be checked.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			st := owner.Verify(p)
			out := c.OutOrStdout()
			if !st.SetUp {
				_, _ = fmt.Fprintln(out, "not bound: nothing to verify")
				return nil
			}
			for _, f := range st.Report.Findings {
				_, _ = fmt.Fprintf(out, "%-9s %s\n", f.Status, f.Name)
			}
			if st.Clean() {
				_, _ = fmt.Fprintln(out, "✓ the seal matches")
				return nil
			}
			return fmt.Errorf("%s; `%s owner reseal` accepts the files as they are", st.Describe(), brand.Command)
		},
	}
}

func newOwnerResealCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reseal",
		Short: "Accept the state files as they are now, and seal them",
		Long: "Use it after a change you made yourself outside authama - an edit to\n" +
			"tokens.json, a restored backup - once `owner verify` has shown what changed.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			st := owner.Verify(p)
			if !st.SetUp {
				return owner.ErrNotSetUp
			}
			if err := requireOwner(c, p, "accepting outside changes"); err != nil {
				return err
			}
			if err := confirmChange(c, "Sealing the files as they are: "+st.Describe()+".", yes); err != nil {
				return err
			}
			if err := owner.Reseal(p, time.Now()); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerReseal, log.Field{Key: "found", Value: st.Describe()})
			say(c, "✓ resealed")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newOwnerForgetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "forget",
		Short: "Unbind the home: remove the key, the seal and the PIN, keep everything else",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			cfg, err := owner.Load(p)
			if err != nil {
				return err
			}
			if cfg == nil {
				return owner.ErrNotSetUp
			}
			if err := requireOwner(c, p, "unbinding the home"); err != nil {
				return err
			}
			if err := confirmChange(c, "The sealing key, the seal and the PIN will be removed. The state files are kept.", yes); err != nil {
				return err
			}
			mutation.LockForMutation(p)
			if err := owner.Forget(p); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerForget)
			say(c, "✓ unbound")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newOwnerPINCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pin",
		Short: "Set, change, reset or remove the PIN",
	}
	set := &cobra.Command{
		Use:   "set",
		Short: "Set a PIN, or change the one set (asks for it first)",
		Long: "A PIN is at least 6 characters: digits, or a passphrase of up to 128. A run such\n" +
			"as 123456 or one repeated character is refused. It is stored only as a salted\n" +
			"Argon2id hash.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			cfg, err := owner.Load(p)
			if err != nil {
				return err
			}
			if cfg == nil {
				return owner.ErrNotSetUp
			}
			current := ""
			if cfg.HasPIN() {
				if current, err = platform.ReadSecret("current PIN: "); err != nil {
					return err
				}
			}
			next, err := readNewPIN()
			if err != nil {
				return err
			}
			mutation.LockForMutation(p)
			if err := owner.SetPIN(p, current, next, time.Now()); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerPINSet)
			say(c, "✓ PIN set")
			return nil
		},
	}
	reset := &cobra.Command{
		Use:   "reset",
		Short: "Replace a forgotten PIN; proves the OS account instead of asking for the old one",
		Long: "Needs no old PIN. It must read the sealing key from this account's own\n" +
			"credential store (on macOS the Keychain may ask for the login password), on the\n" +
			"machine the home was bound on, and it must run in a terminal. The reset is on\n" +
			"the record, and wrong-PIN lockouts are cleared.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if !platform.StdinIsTerminal() {
				return errors.New("a PIN reset is only made at a terminal")
			}
			if err := confirmOrAbort(c, "Reset the PIN for "+p.ConfigDir+"?", false); err != nil {
				return err
			}
			next, err := readNewPIN()
			if err != nil {
				return err
			}
			mutation.LockForMutation(p)
			if err := owner.ResetPIN(p, next, time.Now()); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerPINReset)
			say(c, "✓ PIN reset")
			return nil
		},
	}
	remove := &cobra.Command{
		Use:   "remove",
		Short: "Turn the PIN off (asks for it first)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			current, err := platform.ReadSecret("current PIN: ")
			if err != nil {
				return err
			}
			mutation.LockForMutation(p)
			if err := owner.RemovePIN(p, current, time.Now()); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerPINRemove)
			say(c, "✓ PIN removed")
			return nil
		},
	}
	cmd.AddCommand(set, reset, remove)
	return cmd
}

func newOwnerBiometricCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fingerprint <on|off>",
		Short: "Let a fingerprint stand in for the PIN at the terminal (Touch ID, Windows Hello, fprintd)",
		Long: "The command line asks for a fingerprint first and falls back to the PIN: Touch ID\n" +
			"on a Mac (through the authama-touchid helper, built beside authama), Windows Hello\n" +
			"on Windows (through Windows PowerShell), fprintd on Linux. Not over SSH, where the\n" +
			"prompt would appear on a screen nobody at this terminal sees.",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			on := args[0] == "on"
			if !on && args[0] != "off" {
				return errors.New("say on or off")
			}
			if err := requireOwner(c, p, "changing how you unlock"); err != nil {
				return err
			}
			mutation.LockForMutation(p)
			if err := owner.SetBiometric(p, on, time.Now()); err != nil {
				return err
			}
			log.Audit(p.AuditLog(), eventOwnerBiometric, log.Field{Key: "on", Value: args[0]})
			say(c, "✓ fingerprint unlock %s", args[0])
			return nil
		},
	}
}

func readNewPIN() (string, error) {
	next, err := platform.ReadSecret("new PIN: ")
	if err != nil {
		return "", err
	}
	again, err := platform.ReadSecret("new PIN again: ")
	if err != nil {
		return "", err
	}
	if next != again {
		return "", errors.New("the two PINs differ")
	}
	return next, nil
}

// unlockedHomes remembers, for this process, the homes whose PIN was given, so
// a command that guards more than one step asks once rather than once per step.
var unlockedHomes = map[string]bool{}

// requireOwner asks for the owner's PIN, or a fingerprint, before why. A home
// with no PIN passes. Without a terminal there is nobody to ask, so it
// refuses: a PIN that a pipe could skip would gate nothing.
func requireOwner(c *cobra.Command, p paths.Paths, why string) error {
	cfg, err := owner.Load(p)
	if err != nil {
		return err
	}
	if !cfg.HasPIN() || unlockedHomes[p.ConfigDir] {
		return nil
	}
	if !platform.StdinIsTerminal() {
		return fmt.Errorf("a PIN is set for this home, and %s needs it; run this at a terminal", why)
	}
	errw := c.ErrOrStderr()
	if cfg.Biometric {
		if ok, _ := owner.BiometricAvailable(); ok {
			_, _ = fmt.Fprintf(errw, "%s: touch the fingerprint reader (or wait for the PIN prompt)\n", why)
			if owner.VerifyBiometric(context.Background()) == nil {
				unlockedHomes[p.ConfigDir] = true
				return nil
			}
		}
	}
	for try := 0; try < 3; try++ {
		got, err := platform.ReadSecret(why + " - PIN: ")
		if err != nil {
			return err
		}
		err = owner.CheckPIN(p, got, time.Now())
		if err == nil {
			unlockedHomes[p.ConfigDir] = true
			return nil
		}
		var locked owner.LockedError
		if errors.As(err, &locked) || !errors.Is(err, owner.ErrWrongPIN) {
			return err
		}
		_, _ = fmt.Fprintln(errw, "wrong PIN")
	}
	return owner.ErrWrongPIN
}

// destructiveVerbs are the commands whose effect cannot be taken back from
// inside authama, or that reach servers: with a PIN set, each asks for it.
var destructiveVerbs = map[string]bool{
	"delete": true, "revoke": true, "rotate": true, "rollback": true, "restore": true,
	"prune": true, "clean": true, "recover": true, "deploy": true, "uninstall": true,
	"reset": true,
}

// isDestructive reports whether c is one the PIN gates. `keygen --force`
// replaces a pair in place; `owner pin reset` is how a forgotten PIN is
// recovered, so it cannot itself ask for the PIN.
func isDestructive(c *cobra.Command) bool {
	path := c.CommandPath()
	if strings.Contains(path, " owner ") {
		return false
	}
	if c.Name() == "keygen" {
		f := c.Flags().Lookup("force")
		return f != nil && f.Value.String() == "true"
	}
	if !destructiveVerbs[c.Name()] {
		return false
	}
	if f := c.Flags().Lookup("dry-run"); f != nil && f.Value.String() == "true" {
		return false
	}
	return true
}

// gateDestructive asks for the PIN before a destructive command runs.
func gateDestructive(c *cobra.Command) error {
	if !isDestructive(c) {
		return nil
	}
	p, err := resolvePaths(c)
	if err != nil {
		return nil // the command reports its own path error
	}
	return requireOwner(c, p, strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" "))
}
