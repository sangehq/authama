package owner

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// fakeOSStore stands in for the macOS Keychain or the Secret Service: a file
// in the test's own directory, under the OS store's name, so no test reads or
// writes the real credential store.
type fakeOSStore struct{ path string }

func (fakeOSStore) Name() string { return StoreKeychain }

func (f fakeOSStore) Load() ([]byte, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoKey
	}
	if err != nil {
		return nil, err
	}
	return keyFromHex(strings.TrimSpace(string(b)))
}

func (f fakeOSStore) Save(key []byte) error {
	return os.WriteFile(f.path, []byte(keyHex(key)+"\n"), 0o600)
}

func (f fakeOSStore) Remove() error {
	err := os.Remove(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNoKey
	}
	return err
}

// withFakeOSStore points the orphan check at a fake OS store holding a key.
func withFakeOSStore(t *testing.T) fakeOSStore {
	t.Helper()
	st := fakeOSStore{path: filepath.Join(t.TempDir(), "keychain-item")}
	old := orphanStores
	orphanStores = func(paths.Paths) []Store { return []Store{st} }
	t.Cleanup(func() { orphanStores = old })
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	if err := st.Save(key); err != nil {
		t.Fatal(err)
	}
	return st
}

// Deleting owner.json, seal.json and pin-attempts.json together used to turn
// the seal and the PIN off while the sealing key stayed in the OS keychain
// (audit A-SEC-08). Asked of the OS store, the home is now a seal removed,
// with what to do about it.
func TestAKeyLeftInTheOSStoreIsASealRemoved(t *testing.T) {
	p := home(t)
	st := withFakeOSStore(t)
	if Orphaned(p) {
		t.Fatal("the files alone report it; the test needs the key to be the only trace")
	}
	err := SealRemoved(p)
	var sr *SealRemovedError
	if !errors.As(err, &sr) {
		t.Fatalf("SealRemoved = %v, want a SealRemovedError", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"seal was removed", StoreKeychain,
		ConfigPath(p), SealPath(p), // where to restore them
		"owner init", "owner forget", // re-seal on purpose, or clear it
		"security delete-generic-password -a '" + p.ConfigDir + "' -s " + service,
		"secret-tool clear service " + service + " account '" + p.ConfigDir + "'",
		"owner.key.dpapi",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("the advice does not say %q:\n%s", want, msg)
		}
	}

	// The documented way back: forget removes the orphaned key.
	if err := Forget(p); err != nil {
		t.Fatalf("forget on a key left in the OS store: %v", err)
	}
	if _, err := st.Load(); !errors.Is(err, ErrNoKey) {
		t.Errorf("the key is still in the OS store: %v", err)
	}
	if err := SealRemoved(p); err != nil {
		t.Errorf("still a seal removed after forget: %v", err)
	}
}

// A home never bound is not a seal removed, and neither is one bound and
// sealed as it should be, whatever the OS store holds.
func TestASealedOrNeverBoundHomeIsNotASealRemoved(t *testing.T) {
	p := home(t)
	if err := SealRemoved(p); err != nil {
		t.Fatalf("a never-bound home: %v", err)
	}
	withFakeOSStore(t)
	t.Setenv(StoreEnv, StoreFile)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := SealRemoved(p); err != nil {
		t.Fatalf("a bound and sealed home: %v", err)
	}
	// Bound, with the seal gone: removed too, and resealing is the way back.
	if err := os.Remove(SealPath(p)); err != nil {
		t.Fatal(err)
	}
	err := SealRemoved(p)
	if err == nil || !strings.Contains(err.Error(), "owner reseal") {
		t.Fatalf("a bound home with no seal.json: %v", err)
	}
}
