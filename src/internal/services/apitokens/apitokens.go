// Package apitokens keeps the tokens programs use to call authama's API:
// `ama_` and 32 random bytes, shown once when made. Only a token's SHA-256 is
// kept, in an owner-only file in the state home, so the file holds nothing a
// caller could use.
package apitokens

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Prefix starts every token, so a leaked one is recognisable as authama's.
const Prefix = "ama_"

// ScopeRead reads the home: profiles, keys, hosts, tokens' presence, the
// history and the feed. It is the only scope there is.
const ScopeRead = "read"

// Token is one token's record; never the token itself.
type Token struct {
	Name    string    `json:"name"`
	Hash    string    `json:"hash"` // hex SHA-256 of the token
	Scopes  []string  `json:"scopes"`
	Created time.Time `json:"created"`
	// Expires is the zero time for a token that does not expire.
	Expires  time.Time `json:"expires,omitzero"`
	LastUsed time.Time `json:"last_used,omitzero"`
}

// Expired reports whether t has expired at now.
func (t Token) Expired(now time.Time) bool { return !t.Expires.IsZero() && !now.Before(t.Expires) }

// Allows reports whether t carries scope.
func (t Token) Allows(scope string) bool { return slices.Contains(t.Scopes, scope) }

// File is where the records are kept.
func File(p paths.Paths) string { return filepath.Join(p.StateDir(), "api-tokens.json") }

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ErrNotFound is a name no token has.
var ErrNotFound = errors.New("no API token by that name")

// List is every record, in the order they were made.
func List(p paths.Paths) ([]Token, error) {
	b, err := os.ReadFile(File(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Token
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", File(p), err)
	}
	return out, nil
}

func save(p paths.Paths, toks []Token) error {
	if err := fs.EnsureDir(p.StateDir(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(toks, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteTextAtomic(File(p), string(b)+"\n", 0o600)
}

// Create makes a token named name with the read scope, valid for days (0:
// no expiry), and returns it with its record. The token is returned only
// here: nothing can show it again.
func Create(p paths.Paths, name string, days int, now time.Time) (string, Token, error) {
	if !nameRe.MatchString(name) {
		return "", Token{}, fmt.Errorf("%q is not a token name: use lowercase letters, digits, '.', '_' and '-'", name)
	}
	if days < 0 {
		return "", Token{}, errors.New("a token cannot expire in the past")
	}
	toks, err := List(p)
	if err != nil {
		return "", Token{}, err
	}
	if slices.ContainsFunc(toks, func(t Token) bool { return t.Name == name }) {
		return "", Token{}, fmt.Errorf("an API token named %q exists: revoke it first, or choose another name", name)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Token{}, err
	}
	secret := Prefix + base64.RawURLEncoding.EncodeToString(raw)
	t := Token{Name: name, Hash: hash(secret), Scopes: []string{ScopeRead}, Created: now.UTC()}
	if days > 0 {
		t.Expires = now.UTC().AddDate(0, 0, days)
	}
	if err := save(p, append(toks, t)); err != nil {
		return "", Token{}, err
	}
	return secret, t, nil
}

// Revoke removes the token named name.
func Revoke(p paths.Paths, name string) error {
	toks, err := List(p)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(toks, func(t Token) bool { return t.Name == name })
	if i < 0 {
		return fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	return save(p, slices.Delete(toks, i, i+1))
}

// Check finds the record for secret: a live token with scope. It compares
// hashes in constant time, and records when the token was last used (at most
// once a minute, so a busy caller does not rewrite the file on every call).
func Check(p paths.Paths, secret, scope string, now time.Time) (Token, bool) {
	if !strings.HasPrefix(secret, Prefix) {
		return Token{}, false
	}
	toks, err := List(p)
	if err != nil {
		return Token{}, false
	}
	h := []byte(hash(secret))
	found := -1
	for i, t := range toks {
		if subtle.ConstantTimeCompare(h, []byte(t.Hash)) == 1 {
			found = i
		}
	}
	if found < 0 || toks[found].Expired(now) || !toks[found].Allows(scope) {
		return Token{}, false
	}
	t := toks[found]
	if now.Sub(t.LastUsed) > time.Minute {
		toks[found].LastUsed = now.UTC()
		_ = save(p, toks)
	}
	return t, true
}

func hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
