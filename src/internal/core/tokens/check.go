package tokens

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// The line formats (.npmrc, .netrc, the git credential store) have no
// quoting: a newline in a token starts a new line, and a space in a netrc
// password starts a new keyword. A value like "x\nmachine evil login a
// password b" would plant a second credential. The Python tool wrote such
// values as given; this port refuses them before anything is written.

var npmScope = regexp.MustCompile(`^@[A-Za-z0-9][A-Za-z0-9._~-]*$`)

// Check reports why c cannot be written safely, or nil.
func Check(c Credential) error {
	if c.Host == "" {
		return errors.New("host is empty")
	}
	if hasSpaceOrControl(c.Host) {
		return errors.New("host contains whitespace or control characters")
	}
	for _, f := range [][2]string{
		{"token", c.Token}, {"username", c.Username}, {"password", c.Password},
		{"key", c.Key}, {"secret", c.Secret},
	} {
		if hasControl(f[1]) {
			return errors.New(f[0] + " contains a newline or control character")
		}
	}
	for _, h := range c.Headers {
		if hasControl(h) {
			return errors.New("a header contains a newline or control character")
		}
	}
	switch c.Target {
	case NPM:
		if hasSpaceOrControl(c.Token) {
			return errors.New("npm token contains whitespace")
		}
		if c.Scope != "" && !npmScope.MatchString(c.Scope) {
			return errors.New("npm scope must look like @org (letters, digits, . _ ~ -)")
		}
	case Netrc:
		if hasSpaceOrControl(c.Username) || hasSpaceOrControl(c.Password) {
			return errors.New("netrc login and password cannot contain whitespace")
		}
	case Git:
		if strings.ContainsAny(c.Host, "@") {
			return errors.New("git host cannot contain '@' (put the username in the username field)")
		}
	default:
		if f, ok := formats[c.Target]; ok {
			return f.check(c)
		}
	}
	return nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == 0x2028 || r == 0x2029 }) >= 0
}

func hasSpaceOrControl(s string) bool {
	return hasControl(s) || strings.IndexFunc(s, pySpace) >= 0
}
