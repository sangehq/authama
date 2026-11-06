package settings

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/util/envnames"
)

// Pair is one KEY=value assignment read from a .env file.
type Pair struct {
	Key   string
	Value string
	Line  int
}

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ParseDotenv reads .env syntax: KEY=value lines, blank lines and # comments,
// an optional `export ` prefix, and values that are bare, "double-quoted" (with
// \n, \t, \", \\ and \$ escapes) or 'single-quoted' (literal). A bare value
// ends at a # preceded by whitespace. A line that is none of these is
// returned as a warning naming its line number - never its content, which
// may be a secret - and skipped.
func ParseDotenv(r io.Reader) ([]Pair, []string) {
	var pairs []Pair
	var warns []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimLeft(rest, " \t")
		}
		key, raw, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !keyRe.MatchString(key) {
			warns = append(warns, fmt.Sprintf("line %d: not a KEY=value assignment; skipped", n))
			continue
		}
		trimmed := strings.TrimLeft(raw, " \t")
		// "KEY=   # note" is an empty value and a comment, as in a shell;
		// "KEY=#x" (no space) is the value "#x".
		if trimmed != raw && strings.HasPrefix(trimmed, "#") {
			trimmed = ""
		}
		val, err := parseValue(trimmed)
		if err != nil {
			warns = append(warns, fmt.Sprintf("line %d (%s): %v; skipped", n, key, err))
			continue
		}
		pairs = append(pairs, Pair{Key: key, Value: val, Line: n})
	}
	if err := sc.Err(); err != nil {
		warns = append(warns, "read: "+err.Error())
	}
	return pairs, warns
}

func parseValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch raw[0] {
	case '"':
		var b strings.Builder
		for i := 1; i < len(raw); i++ {
			c := raw[i]
			switch {
			case c == '\\' && i+1 < len(raw):
				i++
				switch raw[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				case 'r':
					b.WriteByte('\r')
				case '"', '\\', '$', '`':
					b.WriteByte(raw[i])
				default:
					b.WriteByte('\\')
					b.WriteByte(raw[i])
				}
			case c == '"':
				return b.String(), trailing(raw[i+1:])
			default:
				b.WriteByte(c)
			}
		}
		return "", errors.New("unterminated double quote")
	case '\'':
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", errors.New("unterminated single quote")
		}
		return raw[1 : end+1], trailing(raw[end+2:])
	}
	v := raw
	for i := 1; i < len(v); i++ {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			v = v[:i]
			break
		}
	}
	return strings.TrimSpace(v), nil
}

// trailing accepts what may follow a closing quote: nothing, or a comment.
func trailing(rest string) error {
	rest = strings.TrimSpace(rest)
	if rest == "" || strings.HasPrefix(rest, "#") {
		return nil
	}
	return errors.New("text after the closing quote")
}

// notFromFiles are the variables a settings file may not set: they decide
// where the settings files themselves are, or whether they are read, so a
// value for them inside one could only be circular.
var notFromFiles = map[string]bool{
	envnames.ConfigDir: true, envnames.DevRoot: true, envnames.NoDotenv: true,
}

// fileForbidden reports whether a key, under any spelling, is one a settings
// file may not set.
func fileForbidden(key string) bool {
	for name := range notFromFiles {
		if key == name {
			return true
		}
		for _, l := range envnames.Legacy(name) {
			if key == l {
				return true
			}
		}
	}
	return false
}

// canonical maps a key to the current name of the setting it spells, or
// returns it unchanged.
func canonical(key string) string {
	for _, name := range envnames.Names() {
		for _, l := range envnames.Legacy(name) {
			if l == key {
				return name
			}
		}
	}
	return key
}

// Env is the process environment, injectable for tests.
type Env struct {
	Lookup func(string) (string, bool)
	Setenv func(string, string) error
}

func (e Env) orOS() Env {
	if e.Lookup == nil {
		e.Lookup = os.LookupEnv
	}
	if e.Setenv == nil {
		e.Setenv = os.Setenv
	}
	return e
}

// DotenvResult is what loading a .env file did.
type DotenvResult struct {
	Path    string
	Present bool
	// Refused is set when the file was not read because others can read it.
	Refused string
	// Set lists the variables taken from the file; Kept those already in the
	// environment, which wins.
	Set, Kept []string
	Warnings  []string
}

// LoadDotenv reads path into the environment without replacing anything
// already set there, under either spelling of a setting. A missing file is
// not an error. On Unix a file that group or others can read is refused, as
// doctor flags it: it is where tokens live. Malformed lines are skipped with a
// warning.
func LoadDotenv(path string, env Env) DotenvResult {
	env = env.orOS()
	res := DotenvResult{Path: path}
	fi, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			res.Warnings = append(res.Warnings, err.Error())
		}
		return res
	}
	res.Present = true
	if !platform.IsWindows() && fi.Mode().Perm()&0o077 != 0 {
		res.Refused = fmt.Sprintf("%s is readable by other users (mode %04o) and holds secrets, so it was not loaded; "+
			"run `chmod 600 %s` or `authama doctor --fix`", path, fi.Mode().Perm(), path)
		return res
	}
	f, err := os.Open(path)
	if err != nil {
		res.Warnings = append(res.Warnings, err.Error())
		return res
	}
	defer func() { _ = f.Close() }()
	pairs, warns := ParseDotenv(f)
	for _, w := range warns {
		res.Warnings = append(res.Warnings, path+": "+w)
	}
	seen := map[string]bool{}
	for _, p := range pairs {
		if p.Value == "" {
			continue
		}
		if fileForbidden(p.Key) {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: line %d: %s cannot be set from this file "+
				"(it decides where the file is or whether it is read); set it in the environment", path, p.Line, p.Key))
			continue
		}
		if seen[p.Key] {
			continue
		}
		seen[p.Key] = true
		if envnames.IsSet(env.Lookup, canonical(p.Key)) || isSet(env.Lookup, p.Key) {
			res.Kept = append(res.Kept, p.Key)
			continue
		}
		if err := env.Setenv(p.Key, p.Value); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s: line %d: %v", path, p.Line, err))
			continue
		}
		res.Set = append(res.Set, p.Key)
	}
	return res
}

func isSet(lookup func(string) (string, bool), k string) bool {
	_, ok := lookup(k)
	return ok
}

// Truthy reads a boolean-ish variable value: 1, true, yes, on.
func Truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
