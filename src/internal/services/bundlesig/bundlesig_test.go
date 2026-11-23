package bundlesig

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/perms"
)

// newKey makes a throwaway ed25519 key with no passphrase in a temp dir.
func newKey(t *testing.T, dir, name string) (key, pub string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not found")
	}
	key = filepath.Join(dir, name)
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", name, "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, out)
	}
	// ssh-keygen refuses a private key others can read, on Windows too.
	if err := perms.SetPerms(key, 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	return key, strings.TrimSpace(string(b))
}

func writeBundle(t *testing.T, dir string) string {
	t.Helper()
	b := filepath.Join(dir, "authama-20261122-120000.age")
	if err := os.WriteFile(b, []byte("age-encryption.org/v1\nnot really\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestASignedBundleIsTrustedOnlyByItsSigners(t *testing.T) {
	dir := t.TempDir()
	key, pub := newKey(t, dir, "me")
	_, otherPub := newKey(t, dir, "them")
	bundle := writeBundle(t, dir)
	signers := filepath.Join(dir, "bundle_signers")

	// Unsigned: fine unless a signature is required.
	if r, err := Check(bundle, Options{Signers: signers}); err != nil || r.Status != Unsigned {
		t.Fatalf("unsigned: %v %v", r, err)
	}
	if _, err := Check(bundle, Options{Signers: signers, Require: true}); err == nil {
		t.Fatal("an unsigned bundle passed a required check")
	}

	if _, err := Sign(bundle, key); err != nil {
		t.Fatal(err)
	}
	// Signed, but no one is trusted yet: intact, untrusted.
	r, err := Check(bundle, Options{Signers: signers})
	if err != nil || r.Status != Untrusted || !strings.HasPrefix(r.Fingerprint, "SHA256:") {
		t.Fatalf("untrusted: %+v %v", r, err)
	}
	if _, err := Check(bundle, Options{Signers: signers, Require: true}); err == nil {
		t.Fatal("an untrusted signer passed a required check")
	}

	// Trusting another key does not trust this one.
	if _, err := Trust(signers, "them@example.test", otherPub); err != nil {
		t.Fatal(err)
	}
	if r, _ := Check(bundle, Options{Signers: signers}); r.Status != Untrusted {
		t.Fatalf("another signer's line trusted the bundle: %+v", r)
	}
	added, err := Trust(signers, "me@example.test", pub)
	if err != nil || !added {
		t.Fatalf("trust: %v %v", added, err)
	}
	if again, _ := Trust(signers, "me@example.test", pub); again {
		t.Error("the same key was added twice")
	}
	r, err = Check(bundle, Options{Signers: signers, Require: true})
	if err != nil || r.Status != Trusted || r.Principal != "me@example.test" {
		t.Fatalf("trusted: %+v %v", r, err)
	}
	if fi, _ := os.Stat(signers); runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("bundle_signers is %v, want owner-only", fi.Mode().Perm())
	}

	// A changed bundle is refused even without Require.
	if err := os.WriteFile(bundle, []byte("age-encryption.org/v1\nswapped\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(bundle, Options{Signers: signers}); err == nil || !strings.Contains(err.Error(), "changed after it was signed") {
		t.Fatalf("a changed bundle: %v", err)
	}
}

// A git commit signature (namespace "git") is not a bundle signature.
func TestAnotherNamespaceIsNotABundleSignature(t *testing.T) {
	dir := t.TempDir()
	key, _ := newKey(t, dir, "me")
	bundle := writeBundle(t, dir)
	if out, err := exec.Command("ssh-keygen", "-q", "-Y", "sign", "-n", "git", "-f", key, bundle).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := Check(bundle, Options{}); err == nil {
		t.Fatal("a git-namespace signature passed as a bundle signature")
	}
}

func TestSignerLineLimitsTheNamespace(t *testing.T) {
	got := SignerLine("me@example.test", "ssh-ed25519 AAAAC3Nz comment here")
	if got != `me@example.test namespaces="authama-bundle" ssh-ed25519 AAAAC3Nz` {
		t.Errorf("line = %q", got)
	}
}
