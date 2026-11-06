// Package pin hashes and checks the owner's PIN or passphrase. It is pure: no
// file, no clock, no randomness of its own - the caller passes the salt.
//
// New hashes are Argon2id at RFC 9106's second recommended setting (t=3,
// 64 MiB, p=4): memory-hard, so a stolen hash costs a GPU far more to guess
// than an iterated hash does. Hashes from earlier releases are
// PBKDF2-HMAC-SHA256 at 600,000 iterations; they still verify, and Verify
// reports them stale so the caller rehashes on the next correct entry. The
// encoded form names its algorithm and cost, so this can change again without
// invalidating a PIN anyone has set.
package pin

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Algorithm is the tag new hashes are encoded under.
const Algorithm = "argon2id"

// The Argon2id cost new hashes are made with: RFC 9106 section 4, the second
// recommended option, for an environment that cannot spare 2 GiB.
const (
	ArgonTime    = 3
	ArgonMemory  = 64 * 1024 // KiB
	ArgonThreads = 4
)

// LegacyAlgorithm is the tag of the hashes earlier releases made; they still
// verify, and are reported stale.
const LegacyAlgorithm = "pbkdf2-sha256"

// Iterations is the PBKDF2 cost earlier releases hashed with.
const Iterations = 600_000

// MinLen and MaxLen bound a PIN or passphrase, in characters.
const (
	MinLen = 6
	MaxLen = 128
)

// SaltLen is the salt length, in bytes, that Hash expects.
const SaltLen = 16

const keyLen = 32

// ErrMalformed is an encoded hash this package cannot read.
var ErrMalformed = errors.New("the stored PIN hash is not in a form authama reads")

// Policy reports why a candidate PIN cannot be used, or nil. Six digits is the
// floor a phone asks for; a passphrase may be anything printable up to 128.
func Policy(p string) error {
	if !utf8.ValidString(p) {
		return errors.New("the PIN is not valid text")
	}
	n := utf8.RuneCountInString(p)
	switch {
	case n < MinLen:
		return fmt.Errorf("the PIN is shorter than %d characters", MinLen)
	case n > MaxLen:
		return fmt.Errorf("the PIN is longer than %d characters", MaxLen)
	}
	if strings.ContainsFunc(p, func(r rune) bool { return unicode.IsControl(r) }) {
		return errors.New("the PIN holds a control character")
	}
	if strings.TrimSpace(p) != p {
		return errors.New("the PIN starts or ends with a space")
	}
	if allSame(p) || sequential(p) {
		return errors.New("the PIN is one repeated character or a run such as 123456; choose one that is harder to guess")
	}
	return nil
}

func allSame(p string) bool {
	first, _ := utf8.DecodeRuneInString(p)
	return strings.Trim(p, string(first)) == ""
}

// sequential reports an ascending or descending run of digits or letters.
func sequential(p string) bool {
	rs := []rune(p)
	up, down := true, true
	for i := 1; i < len(rs); i++ {
		up = up && rs[i] == rs[i-1]+1
		down = down && rs[i] == rs[i-1]-1
	}
	return up || down
}

// Hash returns the encoded hash of p with salt, at the current cost:
// argon2id$v=19$m=65536,t=3,p=4$<salt>$<key>, both base64 (raw, standard
// alphabet), the parameters as the PHC string format writes them.
func Hash(p string, salt []byte) (string, error) {
	if len(salt) < SaltLen {
		return "", fmt.Errorf("the salt is %d bytes; at least %d are needed", len(salt), SaltLen)
	}
	key := argon2.IDKey([]byte(p), salt, ArgonTime, ArgonMemory, ArgonThreads, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$v=%d$m=%d,t=%d,p=%d$%s$%s", Algorithm, argon2.Version, ArgonMemory, ArgonTime, ArgonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// hashPBKDF2 is the hash earlier releases made, for the tests that prove
// those still verify.
func hashPBKDF2(p string, salt []byte, iter int) (string, error) {
	key, err := pbkdf2.Key(sha256.New, p, salt, iter, keyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return LegacyAlgorithm + "$" + strconv.Itoa(iter) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key), nil
}

// Verify reports whether p matches encoded, comparing in constant time. It
// also reports whether the hash is weaker than a new one would be (any
// PBKDF2 hash, or Argon2id below the current cost), so the caller can rehash
// it while it has the PIN in hand.
func Verify(encoded, p string) (ok, stale bool, err error) {
	switch {
	case strings.HasPrefix(encoded, Algorithm+"$"):
		return verifyArgon2id(encoded, p)
	case strings.HasPrefix(encoded, LegacyAlgorithm+"$"):
		ok, err := verifyPBKDF2(encoded, p)
		return ok, true, err
	}
	return false, false, ErrMalformed
}

func verifyArgon2id(encoded, p string) (ok, stale bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[1] != "v="+strconv.Itoa(argon2.Version) {
		return false, false, ErrMalformed
	}
	var m, t, threads int
	if n, err := fmt.Sscanf(parts[2], "m=%d,t=%d,p=%d", &m, &t, &threads); err != nil || n != 3 ||
		m < 8*1024 || m > 4*1024*1024 || t < 1 || t > 100 || threads < 1 || threads > 255 {
		return false, false, ErrMalformed
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[3])
	want, err2 := enc.DecodeString(parts[4])
	if err1 != nil || err2 != nil || len(salt) < SaltLen || len(want) != keyLen {
		return false, false, ErrMalformed
	}
	got := argon2.IDKey([]byte(p), salt, uint32(t), uint32(m), uint8(threads), keyLen) //nolint:gosec // bounded above
	stale = m < ArgonMemory || t < ArgonTime || threads < ArgonThreads
	return subtle.ConstantTimeCompare(got, want) == 1, stale, nil
}

func verifyPBKDF2(encoded, p string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 {
		return false, ErrMalformed
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > 10_000_000 {
		return false, ErrMalformed
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(salt) < SaltLen || len(want) != keyLen {
		return false, ErrMalformed
	}
	got, err := pbkdf2.Key(sha256.New, p, salt, iter, keyLen)
	if err != nil {
		return false, err
	}
	return hmac.Equal(got, want), nil
}
