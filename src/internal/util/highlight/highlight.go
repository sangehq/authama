// Package highlight colours one line of a file by its kind, for display.
//
// It is line-based on purpose: a diff shows lines, not files, and a line of
// JSON, INI, TOML, YAML, a properties file, an ssh_config or a netrc can be
// classified on its own well enough to read. It never decides anything about
// the content - a wrong colour is the worst it can do - and it never builds
// HTML: it returns tokens, and the caller escapes them.
package highlight

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Lang is a file kind the highlighter knows.
type Lang string

const (
	Plain      Lang = ""
	JSON       Lang = "json"
	INI        Lang = "ini" // .pypirc, .gitconfig, git credential files
	TOML       Lang = "toml"
	YAML       Lang = "yaml"
	Properties Lang = "properties"
	SSHConfig  Lang = "ssh"
	Netrc      Lang = "netrc"
	Shell      Lang = "sh"
)

// Kind is a token's class; the CSS names it tok-<kind>.
type Kind string

const (
	Text    Kind = ""
	Key     Kind = "key"
	String  Kind = "str"
	Number  Kind = "num"
	Literal Kind = "lit" // true, false, null, yes, no
	Comment Kind = "com"
	Punct   Kind = "pun"
	Section Kind = "sec" // [section], Host, Match
)

// Token is a run of text and its kind.
type Token struct {
	Kind Kind
	Text string
}

// For names the language of a file from its name.
func For(path string) Lang {
	base := strings.ToLower(filepath.Base(filepath.ToSlash(path)))
	switch {
	case strings.HasSuffix(base, ".json"), base == "config.json", base == "auth.json":
		return JSON
	case base == ".pypirc", base == ".gitconfig", strings.HasSuffix(base, ".ini"), base == "credentials" && strings.Contains(path, ".aws"):
		return INI
	case strings.HasSuffix(base, ".toml"), base == "credentials.toml":
		return TOML
	case strings.HasSuffix(base, ".yml"), strings.HasSuffix(base, ".yaml"), base == "credentials" && strings.Contains(path, ".gem"):
		return YAML
	case strings.HasSuffix(base, ".properties"):
		return Properties
	case base == ".netrc", base == "_netrc":
		return Netrc
	case base == "config" && strings.Contains(filepath.ToSlash(path), ".ssh"), strings.HasSuffix(base, ".conf") && strings.Contains(path, "ssh"):
		return SSHConfig
	case strings.HasSuffix(base, ".sh"), base == ".bashrc", base == ".zshrc", base == ".profile", base == ".env":
		return Shell
	}
	return Plain
}

// Line splits one line into tokens. Concatenating their Text gives the line
// back exactly.
func Line(lang Lang, s string) []Token {
	switch lang {
	case JSON:
		return jsonLine(s)
	case INI, TOML, Properties:
		return keyValueLine(s, lang)
	case YAML:
		return yamlLine(s)
	case SSHConfig, Netrc:
		return wordsLine(s, lang)
	case Shell:
		return shellLine(s)
	}
	return []Token{{Text: s}}
}

// tokens is a builder that merges adjacent runs of one kind.
type tokens []Token

func (t *tokens) add(k Kind, s string) {
	if s == "" {
		return
	}
	if n := len(*t); n > 0 && (*t)[n-1].Kind == k {
		(*t)[n-1].Text += s
		return
	}
	*t = append(*t, Token{Kind: k, Text: s})
}

func jsonLine(s string) []Token {
	var out tokens
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"':
			j := endOfString(s, i)
			str := s[i:j]
			// A string followed by a colon is a key.
			rest := strings.TrimLeft(s[j:], " \t")
			if strings.HasPrefix(rest, ":") {
				out.add(Key, str)
			} else {
				out.add(String, str)
			}
			i = j
		case c == '-' || (c >= '0' && c <= '9'):
			j := i + 1
			for j < len(s) && strings.IndexByte("0123456789.eE+-", s[j]) >= 0 {
				j++
			}
			out.add(Number, s[i:j])
			i = j
		case strings.IndexByte("{}[]:,", c) >= 0:
			out.add(Punct, s[i:i+1])
			i++
		case isWordStart(c):
			j := i
			for j < len(s) && isWordStart(s[j]) {
				j++
			}
			w := s[i:j]
			if w == "true" || w == "false" || w == "null" {
				out.add(Literal, w)
			} else {
				out.add(Text, w)
			}
			i = j
		default:
			out.add(Text, s[i:i+1])
			i++
		}
	}
	return out
}

// endOfString is the index after the closing quote of the string starting at
// i, honouring backslash escapes; an unterminated string runs to the end.
func endOfString(s string, i int) int {
	q := s[i]
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case q:
			return j + 1
		}
	}
	return len(s)
}

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func commentStart(trimmed string, lang Lang) bool {
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	return (lang == INI || lang == Properties) && strings.HasPrefix(trimmed, ";") ||
		lang == Properties && strings.HasPrefix(trimmed, "!")
}

// keyValueLine covers INI, TOML and properties: [section], key = value,
// key: value, and comments.
func keyValueLine(s string, lang Lang) []Token {
	var out tokens
	indent := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	body := s[len(indent):]
	out.add(Text, indent)
	switch {
	case body == "":
	case commentStart(body, lang):
		out.add(Comment, body)
	case strings.HasPrefix(body, "["):
		out.add(Section, body)
	default:
		sep := strings.IndexAny(body, "=:")
		if sep < 0 {
			out.add(Text, body)
			break
		}
		out.add(Key, body[:sep])
		out.add(Punct, body[sep:sep+1])
		value(body[sep+1:], &out)
	}
	return out
}

// value classifies the right-hand side of a key.
func value(v string, out *tokens) {
	lead := v[:len(v)-len(strings.TrimLeft(v, " \t"))]
	out.add(Text, lead)
	rest := v[len(lead):]
	switch {
	case rest == "":
	case rest[0] == '"' || rest[0] == '\'':
		j := endOfString(rest, 0)
		out.add(String, rest[:j])
		out.add(Text, rest[j:])
	case isNumber(rest):
		out.add(Number, rest)
	case isLiteral(rest):
		out.add(Literal, rest)
	default:
		out.add(String, rest)
	}
}

func isNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for i, r := range s {
		if !unicode.IsDigit(r) && !(r == '.' && i > 0) && !(r == '-' && i == 0) {
			return false
		}
	}
	return true
}

func isLiteral(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return true
	}
	return false
}

func yamlLine(s string) []Token {
	var out tokens
	indent := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	body := s[len(indent):]
	out.add(Text, indent)
	if strings.HasPrefix(body, "- ") {
		out.add(Punct, "- ")
		body = body[2:]
	}
	switch {
	case body == "":
	case strings.HasPrefix(body, "#"):
		out.add(Comment, body)
	case body == "---" || body == "...":
		out.add(Punct, body)
	default:
		// A key ends at ": " or a trailing ":"; a quoted key may hold a colon.
		keyEnd := -1
		if body[0] == '"' || body[0] == '\'' {
			j := endOfString(body, 0)
			if strings.HasPrefix(body[j:], ":") {
				keyEnd = j
			}
		} else if i := strings.Index(body, ": "); i > 0 {
			keyEnd = i
		} else if strings.HasSuffix(body, ":") {
			keyEnd = len(body) - 1
		}
		if keyEnd < 0 {
			value(body, &out)
			break
		}
		out.add(Key, body[:keyEnd])
		out.add(Punct, ":")
		value(body[keyEnd+1:], &out)
	}
	return out
}

// wordsLine covers ssh_config and netrc: a keyword, then its arguments.
func wordsLine(s string, lang Lang) []Token {
	var out tokens
	indent := s[:len(s)-len(strings.TrimLeft(s, " \t"))]
	body := s[len(indent):]
	out.add(Text, indent)
	if body == "" {
		return out
	}
	if strings.HasPrefix(body, "#") {
		out.add(Comment, body)
		return out
	}
	fields := splitKeepingSpace(body)
	expectValue := false
	for i, f := range fields {
		if strings.TrimSpace(f) == "" {
			out.add(Text, f)
			continue
		}
		lower := strings.ToLower(f)
		switch {
		case lang == SSHConfig && i == 0 && (lower == "host" || lower == "match"):
			out.add(Section, f)
		case lang == SSHConfig && i == 0:
			out.add(Key, f)
		case lang == Netrc && !expectValue && (lower == "machine" || lower == "default"):
			out.add(Section, f)
			expectValue = lower == "machine"
		case lang == Netrc && !expectValue:
			out.add(Key, f)
			expectValue = true
		case isNumber(f):
			out.add(Number, f)
			expectValue = false
		case isLiteral(f):
			out.add(Literal, f)
			expectValue = false
		default:
			out.add(String, f)
			expectValue = false
		}
	}
	return out
}

func splitKeepingSpace(s string) []string {
	var out []string
	start := 0
	space := s != "" && (s[0] == ' ' || s[0] == '\t')
	for i := 0; i < len(s); i++ {
		sp := s[i] == ' ' || s[i] == '\t'
		if sp != space {
			out = append(out, s[start:i])
			start, space = i, sp
		}
	}
	return append(out, s[start:])
}

func shellLine(s string) []Token {
	var out tokens
	trimmed := strings.TrimLeft(s, " \t")
	indent := s[:len(s)-len(trimmed)]
	out.add(Text, indent)
	if strings.HasPrefix(trimmed, "#") {
		out.add(Comment, trimmed)
		return out
	}
	// NAME=value, optionally after export.
	if strings.HasPrefix(trimmed, "export ") {
		out.add(Section, "export ")
		trimmed = trimmed[len("export "):]
	}
	if i := strings.IndexByte(trimmed, '='); i > 0 && !strings.ContainsAny(trimmed[:i], " \t") {
		out.add(Key, trimmed[:i])
		out.add(Punct, "=")
		value(trimmed[i+1:], &out)
		return out
	}
	out.add(Text, trimmed)
	return out
}

// Check validates a whole file for the languages that can be checked
// without a parser authama does not have: JSON today. It reports whether a
// check ran, and the problem if one did and failed, with its line and column.
func Check(lang Lang, text string) (checked bool, problem string) {
	if lang != JSON || strings.TrimSpace(text) == "" {
		return false, ""
	}
	var v any
	err := json.Unmarshal([]byte(text), &v)
	if err == nil {
		return true, ""
	}
	var se *json.SyntaxError
	if errors.As(err, &se) {
		line, col := position(text, int(se.Offset))
		return true, fmt.Sprintf("line %d, column %d: %s", line, col, se.Error())
	}
	return true, err.Error()
}

// position turns a byte offset into a 1-based line and column.
func position(text string, offset int) (line, col int) {
	if offset > len(text) {
		offset = len(text)
	}
	before := text[:offset]
	line = strings.Count(before, "\n") + 1
	col = offset - strings.LastIndexByte(before, '\n')
	return line, col
}
