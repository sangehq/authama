package renderer

import (
	"strings"
)

// Block is one Host block read back from a managed file: where it sits (its
// profile banner and the heading above it), its names, the comment under its
// Host line and its options in order.
type Block struct {
	Profile  string
	Section  string
	Patterns []string
	Comment  string
	Options  []OptionLine
	// Line is the 1-based line of the Host keyword.
	Line int
	// Git is the `Host github.com` block of git ssh mode.
	Git bool
}

// Alias is the block's first name, the one authama keys a host by.
func (b Block) Alias() string {
	if len(b.Patterns) == 0 {
		return ""
	}
	return b.Patterns[0]
}

// Get is the first value of key (any case), as ssh reads it, and whether it is set.
func (b Block) Get(key string) (string, bool) {
	for _, o := range b.Options {
		if strings.EqualFold(o.Key, key) {
			return o.Value, true
		}
	}
	return "", false
}

// Parsed is a managed file read back: its Host blocks and any comment line
// that belongs to no block (a heading with no host under it).
type Parsed struct {
	Blocks []Block
	Loose  []string
}

// ManagedRegion is the part of content authama owns: a category file whole,
// or in ~/.ssh/config the lines from the managed header to the end marker.
// "" when content has no managed header.
func ManagedRegion(content string) string {
	lines := splitLines(content)
	start, end := -1, -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if start < 0 && isManagedHeader(t) {
			start = i
		} else if start >= 0 && isManagedEnd(t) {
			end = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	if end < 0 {
		end = len(lines) - 1
		for end > start && lines[end] == "" {
			end--
		}
	}
	return strings.Join(lines[start:end+1], "\n") + "\n"
}

// ParseManaged reads the Host blocks of a managed file (or region) back, in
// the shape writeProfile and writeHostBlock write them. The `Host *` block,
// the markers, the banners and the category Includes are authama's own and
// are not returned.
func ParseManaged(content string) Parsed {
	var out Parsed
	profile, git := "", false
	var pending []string
	var cur *Block
	inGlobal := false
	flush := func() {
		if cur != nil {
			out.Blocks = append(out.Blocks, *cur)
			cur = nil
		}
	}
	for i, raw := range splitLines(content) {
		t := strings.TrimSpace(raw)
		indented := raw != t && strings.TrimLeft(raw, " \t") == t && t != ""
		switch {
		case t == "":
			if cur == nil && len(pending) > 0 {
				out.Loose = append(out.Loose, pending...)
			}
			pending = nil
			continue
		case isManagedHeader(t) || isManagedEnd(t) || strings.HasPrefix(t, "# Category: "):
			continue
		case strings.HasPrefix(t, "# --- profile: "):
			flush()
			profile, git, inGlobal = strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(t, "# --- profile: "), "-"), " -"), false, false
			pending = nil
			continue
		case strings.HasPrefix(t, "# --- git ssh mode: "):
			flush()
			git, inGlobal = true, false
			pending = nil
			continue
		}
		if indented && cur != nil {
			if strings.HasPrefix(t, "#") {
				c := strings.TrimSpace(strings.TrimPrefix(t, "#"))
				// The block's first comment is its comment, wherever it sits
				// among the options (one typed in above it moves it down).
				if cur.Comment == "" {
					cur.Comment = c
				} else {
					out.Loose = append(out.Loose, cur.Alias()+": "+t)
				}
				continue
			}
			k, v := splitOption(t)
			cur.Options = append(cur.Options, OptionLine{Key: k, Value: v})
			continue
		}
		if indented && inGlobal {
			continue
		}
		if strings.HasPrefix(t, "#") {
			if cur != nil {
				flush()
			}
			pending = append(pending, strings.TrimSpace(strings.TrimPrefix(t, "#")))
			continue
		}
		k, v := splitOption(t)
		switch strings.ToLower(k) {
		case "host":
			flush()
			pats := strings.Fields(v)
			if len(pats) == 1 && pats[0] == "*" {
				inGlobal = true
				pending = nil
				continue
			}
			inGlobal = false
			cur = &Block{Profile: profile, Patterns: pats, Line: i + 1, Git: git}
			if len(pending) > 0 {
				cur.Section = strings.Join(pending, " ")
			}
			pending = nil
		case "match", "include":
			flush()
			inGlobal = false
		default:
			// A keyword at the left margin still belongs to the block above,
			// as ssh reads it; indentation is only a convention.
			if cur != nil {
				cur.Options = append(cur.Options, OptionLine{Key: k, Value: v})
			}
		}
	}
	flush()
	if len(pending) > 0 {
		out.Loose = append(out.Loose, pending...)
	}
	return out
}

// splitOption splits "Key value" or "Key=value" the way ssh does: on the
// first run of whitespace or a single '='.
func splitOption(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, ""
	}
	k, rest := line[:i], strings.TrimLeft(line[i:], " \t")
	if strings.HasPrefix(rest, "=") {
		rest = strings.TrimLeft(rest[1:], " \t")
	}
	return k, strings.TrimSpace(rest)
}
