// Package bundlesig signs export bundles with an SSH key and checks those
// signatures before a bundle is restored or imported (P-06).
//
// A signature is a detached SSHSIG file next to the bundle, <bundle>.sig,
// written and checked by ssh-keygen -Y in the namespace "authama-bundle", so
// a git commit signature can never stand in for one. Who is trusted to sign is
// an allowed_signers file of its own, bundle_signers in the config home, kept
// apart from git's allowed_signers for the same reason.
package bundlesig

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// Namespace is the SSHSIG namespace every bundle signature is made in.
const Namespace = "authama-bundle"

// SigPath is where a bundle's signature lives.
func SigPath(bundle string) string { return bundle + ".sig" }

// SignersPath is the trusted-signers file: allowed_signers lines, each
// limited to the bundle namespace.
func SignersPath(p paths.Paths) string { return filepath.Join(p.ConfigDir, "bundle_signers") }

// Status is what a check found.
type Status string

const (
	Unsigned  Status = "unsigned"  // no .sig next to the bundle
	Untrusted Status = "untrusted" // a good signature, by a key no signers file names
	Trusted   Status = "trusted"   // a good signature by a trusted signer
)

// Result is a check's answer. Fingerprint and Principal are set once a
// signature was checked.
type Result struct {
	Status      Status
	Principal   string
	Fingerprint string
}

// String is a line for a person.
func (r Result) String() string {
	switch r.Status {
	case Trusted:
		return fmt.Sprintf("signature: good, by %s (%s)", r.Principal, r.Fingerprint)
	case Untrusted:
		return fmt.Sprintf("signature: intact, by %s, a key no bundle_signers line trusts", r.Fingerprint)
	}
	return "signature: none"
}

var keygen = "ssh-keygen"

var fingerprintRe = regexp.MustCompile(`(SHA256:[A-Za-z0-9+/=]+)`)

// Sign signs bundle with the private key at keyPath (ssh-keygen may ask for
// its passphrase, or use the agent), writing SigPath(bundle). An old
// signature is replaced: it belonged to other bytes.
func Sign(bundle, keyPath string) (string, error) {
	if _, err := exec.LookPath(keygen); err != nil {
		return "", errors.New("signing needs ssh-keygen, which is not on PATH")
	}
	if _, err := os.Stat(keyPath); err != nil {
		return "", fmt.Errorf("signing key: %w", err)
	}
	sig := SigPath(bundle)
	if err := os.Remove(sig); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	cmd := exec.Command(keygen, "-q", "-Y", "sign", "-n", Namespace, "-f", keyPath, bundle)
	cmd.Stdin = os.Stdin
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("ssh-keygen -Y sign: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return sig, nil
}

// SignerLine is the bundle_signers line that trusts pubLine as principal.
func SignerLine(principal, pubLine string) string {
	f := strings.Fields(pubLine)
	if len(f) > 2 {
		f = f[:2]
	}
	return fmt.Sprintf("%s namespaces=%q %s", principal, Namespace, strings.Join(f, " "))
}

// Trust adds a SignerLine to the signers file unless that key is already in
// it, and reports whether it added one. The file is owner-only.
func Trust(signers, principal, pubLine string) (bool, error) {
	f := strings.Fields(pubLine)
	if len(f) < 2 {
		return false, errors.New("not a public key line")
	}
	old, err := os.ReadFile(signers)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if bytes.Contains(old, []byte(f[1])) {
		return false, nil
	}
	if len(old) == 0 {
		old = []byte("# Who may sign authama bundles: allowed_signers lines (ssh-keygen(1)).\n")
	} else if !bytes.HasSuffix(old, []byte("\n")) {
		old = append(old, '\n')
	}
	out := append(old, []byte(SignerLine(principal, pubLine)+"\n")...)
	if err := os.MkdirAll(filepath.Dir(signers), 0o700); err != nil {
		return false, err
	}
	return true, os.WriteFile(signers, out, 0o600)
}

// Options say how strict a check is.
type Options struct {
	// Signers is the allowed_signers file; "" or a missing file trusts no one.
	Signers string
	// Require refuses a bundle that is not signed by a trusted signer.
	Require bool
}

// Check checks bundle's signature, if it has one. A signature that does not
// match the bytes, or is made in another namespace, is always an error: the
// bundle was changed after it was signed. A good signature by an untrusted
// key, or no signature, is an error only with Require.
func Check(bundle string, o Options) (Result, error) {
	sig := SigPath(bundle)
	if _, err := os.Stat(sig); errors.Is(err, os.ErrNotExist) {
		if o.Require {
			return Result{Status: Unsigned}, fmt.Errorf("%s is not signed (no %s), and a signature is required", filepath.Base(bundle), filepath.Base(sig))
		}
		return Result{Status: Unsigned}, nil
	} else if err != nil {
		return Result{}, err
	}
	if _, err := exec.LookPath(keygen); err != nil {
		return Result{}, errors.New("the bundle is signed, and checking it needs ssh-keygen, which is not on PATH")
	}
	out, err := keygenOn(bundle, "-Y", "check-novalidate", "-n", Namespace, "-s", sig)
	if err != nil {
		return Result{}, fmt.Errorf("the bundle's signature does not match it: it was changed after it was signed, or the signature is not an authama bundle signature (%s)", out)
	}
	res := Result{Status: Untrusted, Fingerprint: fingerprintRe.FindString(out)}
	if trusted, ok := trustedSigner(bundle, sig, o.Signers); ok {
		res.Status, res.Principal = Trusted, trusted
		return res, nil
	}
	if o.Require {
		where := o.Signers
		if where == "" {
			where = "a signers file"
		}
		return res, fmt.Errorf("the bundle is signed by %s, which %s does not trust; add its line to trust it", res.Fingerprint, where)
	}
	return res, nil
}

// trustedSigner is the principal of signers that verifies the signature.
func trustedSigner(bundle, sig, signers string) (string, bool) {
	if signers == "" {
		return "", false
	}
	if _, err := os.Stat(signers); err != nil {
		return "", false
	}
	out, err := exec.Command(keygen, "-Y", "find-principals", "-n", Namespace, "-s", sig, "-f", signers).Output()
	if err != nil {
		return "", false
	}
	for _, principal := range strings.Fields(string(out)) {
		if _, err := keygenOn(bundle, "-Y", "verify", "-n", Namespace, "-f", signers, "-I", principal, "-s", sig); err == nil {
			return principal, true
		}
	}
	return "", false
}

// keygenOn runs ssh-keygen with the bundle on stdin.
func keygenOn(bundle string, args ...string) (string, error) {
	f, err := os.Open(bundle)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	cmd := exec.Command(keygen, args...)
	cmd.Stdin = f
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
