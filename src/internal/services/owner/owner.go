// Package owner ties an authama home to the person and machine it belongs to.
//
// Three things, all optional and all off until `authama owner init`:
//
//   - A sealing key, kept by the OS for the user (macOS Keychain, the Secret
//     Service on Linux, DPAPI on Windows, else an owner-only file). It never
//     leaves the store except into this process's memory.
//   - A seal over the state files (core/seal), remade after every operation
//     authama performs, so a file changed by anything else, or a home copied
//     to another account or machine, is named the next time authama runs.
//   - A PIN or passphrase, hashed (core/pin), that the web UI asks for at
//     sign-in and again before any change, and that the command line asks
//     for before reading a secret or making a destructive change. A
//     fingerprint can stand in for it where the platform has a reader authama
//     can drive (see biometric_*.go).
//
// Resetting a forgotten PIN needs no old PIN, only this account on this
// machine: the reset must reach the sealing key in the user's own credential
// store, and the binding must match. That is the proof of the OS account.
package owner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/pin"
	"github.com/sangehq/authama/src/internal/core/seal"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ConfigVersion is owner.json's format.
const ConfigVersion = 1

// Config is owner.json in the config home. It holds no secret: the PIN is a
// hash, and the key is in the store it names.
type Config struct {
	Version   int       `json:"version"`
	Binding   string    `json:"binding"`
	Store     string    `json:"store"`
	Created   time.Time `json:"created"`
	PIN       string    `json:"pin,omitempty"`
	PINSet    time.Time `json:"pin_set,omitempty"`
	Biometric bool      `json:"biometric,omitempty"`
}

// HasPIN reports whether a PIN is set.
func (c *Config) HasPIN() bool { return c != nil && c.PIN != "" }

// Errors a caller tells apart.
var (
	ErrNotSetUp  = errors.New("this home is not bound to an owner yet; run `authama owner init`")
	ErrSetUp     = errors.New("this home is already bound to an owner; `authama owner status` shows it")
	ErrNoPIN     = errors.New("no PIN is set; `authama owner pin set` sets one")
	ErrWrongPIN  = errors.New("that PIN is not the one set for this home")
	ErrElsewhere = errors.New("this home was bound on another account or machine; " +
		"`authama owner forget` and `authama owner init` bind it here")
)

// ConfigPath is owner.json.
func ConfigPath(p paths.Paths) string { return filepath.Join(p.ConfigDir, "owner.json") }

// SealPath is the seal record, in the state home.
func SealPath(p paths.Paths) string { return filepath.Join(p.StateDir(), "seal.json") }

func attemptsPath(p paths.Paths) string { return filepath.Join(p.StateDir(), "pin-attempts.json") }

// SealedFiles names the files a seal covers: everything authama keeps that a
// change behind its back would make it act on, or that holds a credential.
// The history database is not here: it changes on every read of a secret and
// carries its own content hashes.
func SealedFiles(p paths.Paths) map[string]string {
	return map[string]string{
		"manifest.json":    p.Manifest(),
		"inventory.json":   p.Inventory(),
		"revoked.json":     inventory.RevokedPath(p.Inventory()),
		"retired.json":     inventory.RetiredPath(p.Inventory()),
		"providers.json":   p.Providers(),
		"tokens.json":      p.Tokens(),
		"authama.json":     p.SettingsFile(),
		".env":             p.EnvFile(),
		"age-identity.txt": p.AgeIdentity(),
		"owner.json":       ConfigPath(p),
	}
}

// Load reads owner.json. A home with none returns (nil, nil).
func Load(p paths.Paths) (*Config, error) {
	b, err := os.ReadFile(ConfigPath(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigPath(p), err)
	}
	if c.Version != ConfigVersion {
		return nil, fmt.Errorf("%s: format %d, this authama reads %d", ConfigPath(p), c.Version, ConfigVersion)
	}
	return &c, nil
}

func save(p paths.Paths, c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := fs.EnsureDir(p.ConfigDir, 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(ConfigPath(p), string(b)+"\n", 0o600)
}

// CurrentBinding is the account and machine this process runs as.
func CurrentBinding() (seal.Binding, error) {
	u, err := user.Current()
	if err != nil {
		return seal.Binding{}, fmt.Errorf("who is running authama: %w", err)
	}
	m, err := MachineID()
	if err != nil {
		return seal.Binding{}, fmt.Errorf("which machine this is: %w", err)
	}
	return seal.Binding{User: u.Username, UID: u.Uid, Machine: m}, nil
}

// Init binds the home: a new key in the store, owner.json, and a first seal.
func Init(p paths.Paths, now time.Time) (*Config, error) {
	if c, err := Load(p); err != nil {
		return nil, err
	} else if c != nil {
		return nil, ErrSetUp
	}
	b, err := CurrentBinding()
	if err != nil {
		return nil, err
	}
	st := StoreFor(p)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	if err := st.Save(key); err != nil {
		return nil, fmt.Errorf("keeping the sealing key in %s: %w", st.Name(), err)
	}
	c := &Config{Version: ConfigVersion, Binding: b.ID(), Store: st.Name(), Created: now.UTC()}
	if err := save(p, c); err != nil {
		// No owner.json means no binding: take the key back out, so a failed
		// init leaves nothing behind that a later one would trip over.
		_ = st.Remove()
		return nil, err
	}
	if err := sealWith(p, key, b, now); err != nil {
		return c, fmt.Errorf("bound, but the first seal was not written (%w); `authama owner reseal` writes it", err)
	}
	return c, nil
}

// Forget unbinds the home: the key leaves the store, owner.json and the seal
// are removed, and with them the PIN. The state files themselves are kept.
func Forget(p paths.Paths) error {
	c, err := Load(p)
	if err != nil {
		return err
	}
	if c == nil {
		if Orphaned(p) {
			return finishForget(p)
		}
		return ErrNotSetUp
	}
	// owner.json goes first: without it the home is unbound, so a forget
	// that stops part-way leaves a home that works without a PIN, never one
	// that asks for a PIN it can no longer reset because the key is gone.
	if err := os.Remove(ConfigPath(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, f := range []string{SealPath(p), attemptsPath(p)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	st := storeNamed(p, c.Store)
	if err := st.Remove(); err != nil && !errors.Is(err, ErrNoKey) {
		return fmt.Errorf("unbound, but the sealing key was not removed from %s: %w", st.Name(), err)
	}
	return nil
}

// unlock loads the config and the key, and checks this is the bound account
// and machine. Every PIN change goes through it.
func unlock(p paths.Paths) (*Config, []byte, seal.Binding, error) {
	c, err := Load(p)
	if err != nil {
		return nil, nil, seal.Binding{}, err
	}
	if c == nil {
		return nil, nil, seal.Binding{}, ErrNotSetUp
	}
	b, err := CurrentBinding()
	if err != nil {
		return nil, nil, seal.Binding{}, err
	}
	if b.ID() != c.Binding {
		return nil, nil, seal.Binding{}, ErrElsewhere
	}
	key, err := storeNamed(p, c.Store).Load()
	if err != nil {
		return nil, nil, seal.Binding{}, fmt.Errorf("reading the sealing key from %s: %w", c.Store, err)
	}
	return c, key, b, nil
}

// Recoverable reports why a forgotten PIN could not be reset here, or nil:
// the reset reads the sealing key from this account's store on the bound
// machine, so both must hold before a PIN is set, or setting one could lock
// the owner out for good.
func Recoverable(p paths.Paths) error {
	_, _, _, err := unlock(p)
	if err != nil {
		return fmt.Errorf("a forgotten PIN could not be reset: %w", err)
	}
	return nil
}

// SetPIN sets the PIN, or changes it: with one set, current must match it.
func SetPIN(p paths.Paths, current, next string, now time.Time) error {
	c, key, b, err := unlock(p)
	if err != nil {
		return err
	}
	if c.HasPIN() {
		if err := CheckPIN(p, current, now); err != nil {
			return err
		}
	}
	return writePIN(p, c, key, b, next, now)
}

// ResetPIN replaces a forgotten PIN. No old PIN: reaching the key in this
// account's credential store, on the bound machine, is the proof.
func ResetPIN(p paths.Paths, next string, now time.Time) error {
	c, key, b, err := unlock(p)
	if err != nil {
		return err
	}
	_ = os.Remove(attemptsPath(p))
	return writePIN(p, c, key, b, next, now)
}

// RemovePIN turns the PIN off; current must match it.
func RemovePIN(p paths.Paths, current string, now time.Time) error {
	c, key, b, err := unlock(p)
	if err != nil {
		return err
	}
	if !c.HasPIN() {
		return ErrNoPIN
	}
	if err := CheckPIN(p, current, now); err != nil {
		return err
	}
	c.PIN, c.PINSet, c.Biometric = "", time.Time{}, false
	if err := save(p, c); err != nil {
		return err
	}
	return sealWith(p, key, b, now)
}

// SetBiometric turns fingerprint unlock on or off. It needs a PIN to fall
// back on, and, to turn it on, a reader authama can use.
func SetBiometric(p paths.Paths, on bool, now time.Time) error {
	c, key, b, err := unlock(p)
	if err != nil {
		return err
	}
	if on && !c.HasPIN() {
		return errors.New("set a PIN first: a fingerprint stands in for it, and it is what is asked for when the reader fails")
	}
	if on {
		if ok, why := BiometricAvailable(); !ok {
			return fmt.Errorf("no fingerprint unlock on this machine: %s", why)
		}
	}
	c.Biometric = on
	if err := save(p, c); err != nil {
		return err
	}
	return sealWith(p, key, b, now)
}

func writePIN(p paths.Paths, c *Config, key []byte, b seal.Binding, next string, now time.Time) error {
	if err := pin.Policy(next); err != nil {
		return err
	}
	salt := make([]byte, pin.SaltLen)
	_, _ = rand.Read(salt)
	enc, err := pin.Hash(next, salt)
	if err != nil {
		return err
	}
	c.PIN, c.PINSet = enc, now.UTC()
	if err := save(p, c); err != nil {
		return err
	}
	return sealWith(p, key, b, now)
}

// read reads the sealed files; an absent one is nil.
func read(p paths.Paths) (map[string][]byte, error) {
	out := map[string][]byte{}
	for name, path := range SealedFiles(p) {
		b, err := os.ReadFile(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			out[name] = nil
		case err != nil:
			return nil, err
		default:
			if b == nil {
				b = []byte{}
			}
			out[name] = b
		}
	}
	return out, nil
}

func sealWith(p paths.Paths, key []byte, b seal.Binding, now time.Time) error {
	files, err := read(p)
	if err != nil {
		return err
	}
	r, err := seal.Make(key, b, files, now)
	if err != nil {
		return err
	}
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := fs.EnsureDir(p.StateDir(), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(SealPath(p), string(out)+"\n", 0o600)
}

// Reseal remakes the seal from the files as they are now. authama calls it
// after an operation it performed; `authama owner reseal` calls it to accept
// a change made outside authama. A home with no owner is left alone.
func Reseal(p paths.Paths, now time.Time) error {
	c, err := Load(p)
	if err != nil || c == nil {
		return err
	}
	b, err := CurrentBinding()
	if err != nil {
		return err
	}
	if b.ID() != c.Binding {
		// Never rebind silently: that would bless a home copied from elsewhere.
		return ErrElsewhere
	}
	key, err := storeNamed(p, c.Store).Load()
	if err != nil {
		return err
	}
	return sealWith(p, key, b, now)
}

// State is what Verify found.
type State struct {
	SetUp  bool
	Config *Config
	Report seal.Report
	// Problem is why the seal could not be checked at all - the key could not
	// be read, or the seal file is missing - as opposed to a check that ran
	// and found differences.
	Problem string
}

// Clean reports a home with no owner, or one whose seal checks out.
func (s State) Clean() bool { return !s.SetUp || (s.Problem == "" && s.Report.Clean()) }

// Verify checks the files against the seal. It never changes anything.
func Verify(p paths.Paths) State {
	c, err := Load(p)
	if err != nil {
		return State{SetUp: true, Problem: err.Error()}
	}
	if c == nil {
		if Orphaned(p) {
			return State{SetUp: true, Problem: ErrOrphaned.Error()}
		}
		return State{}
	}
	st := State{SetUp: true, Config: c}
	b, err := CurrentBinding()
	if err != nil {
		st.Problem = err.Error()
		return st
	}
	key, err := storeNamed(p, c.Store).Load()
	if err != nil {
		st.Problem = fmt.Sprintf("the sealing key could not be read from %s (%v): this account cannot check the seal", c.Store, err)
		return st
	}
	raw, err := os.ReadFile(SealPath(p))
	if err != nil {
		st.Problem = "the seal is missing (" + SealPath(p) + "); `authama owner reseal` makes a new one from the files as they are"
		return st
	}
	var r seal.Record
	if err := json.Unmarshal(raw, &r); err != nil {
		st.Problem = "the seal is unreadable: " + err.Error()
		return st
	}
	files, err := read(p)
	if err != nil {
		st.Problem = err.Error()
		return st
	}
	st.Report = seal.Verify(key, b, r, files)
	return st
}

// Describe is a one-paragraph account of a State that is not clean.
func (s State) Describe() string {
	switch {
	case s.Clean():
		return "the seal matches"
	case s.Problem != "":
		return s.Problem
	case !s.Report.Authentic:
		return "the seal itself was changed, or was made with another key"
	case !s.Report.Bound:
		return "this home was sealed on another account or machine"
	}
	msg := ""
	for i, f := range s.Report.Findings {
		if i > 0 {
			msg += ", "
		}
		msg += f.Name + " " + string(f.Status)
	}
	return msg + " since authama last sealed it"
}

// Platform names the OS family, for messages.
func Platform() string { return runtime.GOOS }

// keyHex and keyFromHex are how every store spells the key.
func keyHex(k []byte) string { return hex.EncodeToString(k) }

func keyFromHex(s string) ([]byte, error) {
	k, err := hex.DecodeString(s)
	if err != nil || len(k) != 32 {
		return nil, errors.New("the stored sealing key is not 32 bytes of hex")
	}
	return k, nil
}

// ErrOrphaned is a check on a home whose owner.json is gone while the seal,
// the attempt count or a file-store key is still there.
var ErrOrphaned = errors.New("owner.json is missing, but this home was bound: restore owner.json, or run `authama owner forget` to finish unbinding")

// Orphaned reports a home that lost owner.json but kept what binding left:
// a forget that stopped part-way, or somebody deleting the file to switch the
// seal and the PIN off. Only files are consulted, so it is cheap enough for
// every web request and never prompts an OS key store.
func Orphaned(p paths.Paths) bool {
	if _, err := os.Lstat(ConfigPath(p)); !errors.Is(err, os.ErrNotExist) {
		return false
	}
	for _, f := range []string{SealPath(p), attemptsPath(p), fileStore{p}.path()} {
		if _, err := os.Lstat(f); err == nil {
			return true
		}
	}
	return false
}

// finishForget removes what a binding left once owner.json is gone. The key
// is taken out of whichever store it could be in; a store that has none is
// not an error.
func finishForget(p paths.Paths) error {
	for _, f := range []string{SealPath(p), attemptsPath(p)} {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, st := range []Store{fileStore{p}, StoreFor(p)} {
		if err := st.Remove(); err != nil && !errors.Is(err, ErrNoKey) {
			return fmt.Errorf("unbound, but the sealing key was not removed from %s: %w", st.Name(), err)
		}
	}
	return nil
}
