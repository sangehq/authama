package tokens

import (
	"fmt"
	"regexp"
	"strings"
)

// The token config stores the names of environment variables, never secrets,
// which is what makes it safe to show in a page and to commit. Nothing in the
// parser enforces that - a hand-edited file can hold a literal token anywhere -
// so the web UI shows and saves the file only after CheckNamesOnly passes.

// envName is an environment variable name as they are written: upper case,
// digits and underscores, and short. POSIX allows lower case too, but a real
// token is mixed case far more often than a variable is - a GitHub classic
// token, "ghp_" and 36 letters and digits, is a valid POSIX name - so lower
// case is refused, and so is anything long enough to be a token.
var envName = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,47}$`)

// tokenPrefixes start well-known credentials; a "name" that starts with one,
// in any case, is a token pasted where its variable belongs.
var tokenPrefixes = []string{"GHP_", "GHO_", "GHU_", "GHS_", "GHR_", "GITHUB_PAT_", "GLPAT-", "NPM_", "XOX", "SK_LIVE_", "SK_TEST_", "AKIA", "PYPI-"}

// isEnvName reports whether s is a variable name and not a token.
func isEnvName(s string) bool {
	if !envName.MatchString(s) {
		return false
	}
	for _, p := range tokenPrefixes {
		if strings.HasPrefix(s, p) && len(s) > len(p)+8 {
			return false
		}
	}
	return true
}

// headerRef is a ${VAR} reference inside a header value.
var headerRef = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)

// secretHeader names a header whose value is a credential.
var secretHeader = regexp.MustCompile(`(?i)auth|token|key|secret|pass|cookie|session`)

// MaxConfigBytes bounds a token config the web UI accepts; the shipped
// all-targets example is a few kilobytes.
const MaxConfigBytes = 256 << 10

// CheckNamesOnly reports why text is not a token config that holds names only,
// or nil. It parses the config, and then checks every entry: a field named
// "env" or ending in "_env" must be a variable name, and a header that carries
// a credential must take its value from a ${VAR}. Free text - "_help", a
// service or type, a scope, a header like "X-Org: acme" - is allowed.
func CheckNamesOnly(text string) error {
	if len(text) > MaxConfigBytes {
		return fmt.Errorf("the token config is %d bytes, over the %d this accepts", len(text), MaxConfigBytes)
	}
	c, err := ParseConfig(text)
	if err != nil {
		return err
	}
	var problems []string
	walk(c.root, nil, func(path []string, v any) {
		if len(path) == 0 {
			return
		}
		key := path[len(path)-1]
		where := strings.Join(path, ".")
		switch {
		case key == "env" || strings.HasSuffix(key, "_env"):
			s, ok := v.(string)
			if !ok || !isEnvName(s) {
				problems = append(problems, fmt.Sprintf("%s must name an environment variable, not hold a value", where))
			}
		case len(path) >= 2 && path[len(path)-2] == "headers":
			s, _ := v.(string)
			name, value, _ := strings.Cut(s, ":")
			if secretHeader.MatchString(name) && !headerRef.MatchString(value) {
				problems = append(problems, fmt.Sprintf("%s (%s) carries a credential: take it from ${VAR}", where, strings.TrimSpace(name)))
			}
		}
	})
	if len(problems) > 0 {
		return fmt.Errorf("the token config holds more than variable names: %s", strings.Join(problems, "; "))
	}
	return nil
}

// walk visits every leaf under v with the keys, and list indexes, that lead
// to it: a header is visited as ["composer", "<host>", "headers", "0"].
func walk(v any, path []string, leaf func([]string, any)) {
	switch x := v.(type) {
	case *object:
		for _, k := range x.keys {
			walk(x.vals[k], append(append([]string{}, path...), k), leaf)
		}
	case []any:
		for i, item := range x {
			walk(item, append(append([]string{}, path...), fmt.Sprint(i)), leaf)
		}
	default:
		leaf(path, v)
	}
}
