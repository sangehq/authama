//go:build darwin

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

// The real macOS Keychain, end to end (audit A-SEC-08, the keychain case the
// e2e plan listed as missing). Every other owner test uses a stand-in store, so
// this one alone touches the login keychain: it runs only when asked
// (AUTHAMA_TEST_KEYCHAIN=1), under an account no real home has (a temp
// directory), and removes its item however it ends.
func TestTheKeychainStoreAndASealRemovedWithTheKeyLeftThere(t *testing.T) {
	if os.Getenv("AUTHAMA_TEST_KEYCHAIN") != "1" {
		t.Skip("set AUTHAMA_TEST_KEYCHAIN=1 to test against the real login keychain")
	}
	t.Setenv(StoreEnv, "")
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	st := osStore(p)
	if st == nil || st.Name() != StoreKeychain {
		t.Fatalf("osStore = %v, want the keychain", st)
	}
	t.Cleanup(func() { _ = st.Remove() })

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(key); err != nil {
		t.Fatalf("saving to the keychain: %v", err)
	}
	got, err := st.Load()
	if err != nil {
		t.Fatalf("reading the key back: %v", err)
	}
	if string(got) != string(key) {
		t.Fatal("the keychain returned a different key")
	}

	// owner.json, seal.json and pin-attempts.json are gone; only the key is
	// left, in the keychain. Orphaned reads files only and cannot see it;
	// SealRemoved asks the keychain and must.
	if Orphaned(p) {
		t.Fatal("Orphaned saw a key it is not meant to look for")
	}
	err = SealRemoved(p)
	var sr *SealRemovedError
	if !errors.As(err, &sr) || !strings.Contains(sr.Detail, StoreKeychain) {
		t.Fatalf("SealRemoved = %v, want the key left in the keychain named", err)
	}
	if !strings.Contains(sr.Advice(), "security delete-generic-password -a") {
		t.Errorf("the advice does not give the keychain command:\n%s", sr.Advice())
	}

	// Forget finishes it: the key leaves the keychain, and the home is clean.
	if err := Forget(p); err != nil {
		t.Fatalf("forget: %v", err)
	}
	if _, err := st.Load(); !errors.Is(err, ErrNoKey) {
		t.Errorf("after forget the keychain still answers: %v", err)
	}
	if err := SealRemoved(p); err != nil {
		t.Errorf("after forget SealRemoved still reports: %v", err)
	}
}
