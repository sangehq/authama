// Package pin hashes and checks the owner's PIN or passphrase. It is pure: no
// file, no clock, no randomness of its own - the caller passes the salt.
//
// The hash is PBKDF2-HMAC-SHA256 at 600,000 iterations, OWASP's figure for that
// function, from the standard library. Argon2id would need golang.org/x/crypto,
// and the binary links nothing that is not a recorded decision (see
// cmd/authama's layering test), so the encoded form names its algorithm and
// its cost: a later release can add argon2id beside it and rehash on the next
// successful check, without invalidating a PIN anyone has set.
package pin

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Algorithm is the tag the current hash is encoded under.
const Algorithm = "pbkdf2-sha256"

// Iterations is the PBKDF2 cost new hashes are made with.
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
// pbkdf2-sha256$600000$<salt>$<key>, both base64 (raw, standard alphabet).
func Hash(p string, salt []byte) (string, error) {
	if len(salt) < SaltLen {
		return "", fmt.Errorf("the salt is %d bytes; at least %d are needed", len(salt), SaltLen)
	}
	return encode(Iterations, salt, p)
}

func encode(iter int, salt []byte, p string) (string, error) {
	key, err := pbkdf2.Key(sha256.New, p, salt, iter, keyLen)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return Algorithm + "$" + strconv.Itoa(iter) + "$" + enc.EncodeToString(salt) + "$" + enc.EncodeToString(key), nil
}

// Verify reports whether p matches encoded, comparing in constant time. It
// also reports whether the hash is weaker than a new one would be, so the
// caller can rehash it while it has the PIN in hand.
func Verify(encoded, p string) (ok, stale bool, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != Algorithm {
		return false, false, ErrMalformed
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > 10_000_000 {
		return false, false, ErrMalformed
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil || len(salt) < SaltLen || len(want) != keyLen {
		return false, false, ErrMalformed
	}
	got, err := pbkdf2.Key(sha256.New, p, salt, iter, keyLen)
	if err != nil {
		return false, false, err
	}
	return hmac.Equal(got, want), iter < Iterations, nil
}
