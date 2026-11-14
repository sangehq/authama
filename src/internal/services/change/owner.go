package change

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sangehq/authama/src/internal/core/pin"
	"github.com/sangehq/authama/src/internal/services/owner"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// The owner binding from the web UI. Every one acts outside the files a
// preview copies - the OS key store, the seal - so it does its work in
// OnApply and says what it will do in Effects, and each Effects checks first
// what would make the change fail or lock the owner out, so the preview says
// so instead of the confirmation. None carries a PIN in its Args: a PIN
// reaches here from the web's secret stash, and is never shown.

// OwnerInit is `authama owner init`.
func OwnerInit() Change {
	return Change{
		Command: "owner init", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			if c, err := owner.Load(p); err != nil {
				return nil, err
			} else if c != nil {
				return nil, owner.ErrSetUp
			}
			if _, err := owner.CurrentBinding(); err != nil {
				return nil, err
			}
			return []string{
				"keeps a new sealing key in " + owner.StoreFor(p).Name(),
				"binds this home to this account and machine, and seals its state files",
				"sets no PIN: set one next, from this page",
			}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			c, err := owner.Init(p, time.Now())
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "bound; the key is in %s\n", c.Store)
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// OwnerPINSet is `authama owner pin set`: a new PIN, or a changed one when
// current is the PIN set now. It refuses a PIN that could not be reset if
// forgotten, and a PIN the policy refuses, at preview.
func OwnerPINSet(current, next, confirm string) Change {
	return Change{
		Command: "owner pin set", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			c, err := owner.Load(p)
			if err != nil {
				return nil, err
			}
			if c == nil {
				return nil, owner.ErrNotSetUp
			}
			if next != confirm {
				return nil, errors.New("the two new PINs differ")
			}
			if err := pin.Policy(next); err != nil {
				return nil, err
			}
			if c.HasPIN() && current == "" {
				return nil, errors.New("a PIN is set: give the current one to change it")
			}
			// The guard against locking yourself out: only set a PIN that
			// `owner pin reset` could replace if it is forgotten.
			if err := owner.Recoverable(p); err != nil {
				return nil, err
			}
			verb := "sets a PIN"
			if c.HasPIN() {
				verb = "changes the PIN"
			}
			return []string{
				verb + ": the web UI asks for it at sign-in and before changes; the command line before secrets and destructive commands",
				"stores only its salted hash",
				"if it is forgotten: run `authama owner pin reset` at a terminal on this machine",
			}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			if err := owner.SetPIN(p, current, next, time.Now()); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "PIN set")
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// OwnerPINRemove is `authama owner pin remove`.
func OwnerPINRemove(current string) Change {
	return Change{
		Command: "owner pin remove", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			c, err := owner.Load(p)
			if err != nil {
				return nil, err
			}
			if !c.HasPIN() {
				return nil, owner.ErrNoPIN
			}
			if current == "" {
				return nil, errors.New("give the current PIN to remove it")
			}
			return []string{"turns the PIN off: sign-in and changes no longer ask for one", "turns fingerprint unlock off with it"}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			if err := owner.RemovePIN(p, current, time.Now()); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "PIN removed")
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// OwnerReseal is `authama owner reseal --yes`: accept the files as they are.
func OwnerReseal() Change {
	return Change{
		Command: "owner reseal", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			st := owner.Verify(p)
			if !st.SetUp {
				return nil, owner.ErrNotSetUp
			}
			return []string{"accepts the state files as they are now, and seals them: " + st.Describe()}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			if err := owner.Reseal(p, time.Now()); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "resealed")
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// OwnerForget is `authama owner forget --yes`. confirmed is the word the form
// asks to be typed, so the binding is not dropped by a stray click.
func OwnerForget(confirmed bool) Change {
	return Change{
		Command: "owner forget", NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			c, err := owner.Load(p)
			if err != nil {
				return nil, err
			}
			if c == nil {
				return nil, owner.ErrNotSetUp
			}
			if !confirmed {
				return nil, errors.New(`type "forget" to confirm`)
			}
			return []string{
				"removes owner.json, the seal and the PIN, then the sealing key from " + c.Store,
				"keeps every other file: the manifest, keys, tokens and history stay",
			}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			if err := owner.Forget(p); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, "unbound")
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}

// OwnerFingerprint is `authama owner fingerprint on|off`.
func OwnerFingerprint(on bool) Change {
	word := map[bool]string{true: "on", false: "off"}[on]
	return Change{
		Command: "owner fingerprint", Args: []string{word}, NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			c, err := owner.Load(p)
			if err != nil {
				return nil, err
			}
			if c == nil {
				return nil, owner.ErrNotSetUp
			}
			if on && !c.HasPIN() {
				return nil, errors.New("set a PIN first: a fingerprint stands in for it")
			}
			if on {
				if ok, why := owner.BiometricAvailable(); !ok {
					return nil, errors.New(why)
				}
			}
			return []string{"turns fingerprint unlock " + word + " at the terminal; the web UI always asks for the PIN"}, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			if err := owner.SetBiometric(p, on, time.Now()); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "fingerprint unlock %s\n", word)
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}
