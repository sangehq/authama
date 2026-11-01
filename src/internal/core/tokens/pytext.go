package tokens

import (
	"strings"
	"unicode"
)

// The file formats are line- and token-oriented, and which line counts as a
// line, and which character as a blank, decides what a merge keeps. These
// mirror Python's str.splitlines / str.split / str.strip so a file is cut up
// exactly as the tool it replaces cut it.

// pySpace is Python's str.isspace for one rune.
func pySpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

func pyStrip(s string) string { return strings.TrimFunc(s, pySpace) }

func pyFields(s string) []string { return strings.FieldsFunc(s, pySpace) }

// pySplitLines is str.splitlines(): the line breaks are \n, \r\n, \r, \v, \f,
// \x1c-\x1e, \x85, U+2028 and U+2029, and a trailing break adds no empty line.
func pySplitLines(s string) []string {
	var out []string
	rs := []rune(s)
	var cur []rune
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch r {
		case '\r':
			out = append(out, string(cur))
			cur = cur[:0]
			if i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			out = append(out, string(cur))
			cur = cur[:0]
		default:
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

// joinStripNL is "\n".join(lines).strip("\n").
func joinStripNL(lines []string) string { return strings.Trim(strings.Join(lines, "\n"), "\n") }
