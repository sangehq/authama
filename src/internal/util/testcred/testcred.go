// Package testcred hands tests the tokens, passwords and passphrases they need,
// generated when the test binary runs rather than written in the source.
//
// A secret scanner cannot tell a test value from a real one and does not try:
// it matches on shape (a ghp_ prefix, a long mixed string) and on name (any
// literal under something called password, secret or token). Each such literal
// is an incident somebody has to open and close by hand. A value that only
// exists at run time gives the scanner nothing to read.
//
// Values are memoised per purpose, so every assertion in one run sees the same
// value for the same purpose, and distinct purposes get distinct values. Name
// each purpose with one plain lowercase word: a scanner reads a compound string
// in that position as the secret itself.
//
// Only tests import this package, so it is never linked into the binary.
package testcred

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

var (
	mu     sync.Mutex
	values = map[string]string{}
)

// Secret returns an opaque value for purpose: 32 lowercase hex characters,
// the same for the whole run.
func Secret(purpose string) string {
	mu.Lock()
	defer mu.Unlock()
	if v, ok := values[purpose]; ok {
		return v
	}
	b := make([]byte, 16)
	// crypto/rand.Read never returns an error since Go 1.24; it aborts the
	// program itself if the system source fails.
	_, _ = rand.Read(b)
	v := hex.EncodeToString(b)
	values[purpose] = v
	return v
}

// Token returns Secret(purpose) behind a provider prefix, e.g. Token("ghp",
// "ci") is "ghp_<hex>", for code that recognises a token by its prefix.
func Token(prefix, purpose string) string {
	return fmt.Sprintf("%s_%s", prefix, Secret(purpose))
}
