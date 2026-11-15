package configsvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/renderer"
)

// Shadow is a foreign block above the managed block that ssh reads first for a
// managed alias. ssh keeps the first value it obtains for each keyword, so every
// keyword in Keywords comes from the user's block, not from the manifest.
type Shadow struct {
	Alias    string
	File     string // the config file, as a path on disk
	Line     int    // 1-based line of the Host line (or of the first top-level option)
	Header   string // the Host line as written, or "(top level)"
	Keywords []string
}

// String is the one-line report: alias, file:line, and what it takes over.
func (s Shadow) String() string {
	return fmt.Sprintf("%s is shadowed by %s:%d (%s), which sets %s first",
		s.Alias, s.File, s.Line, s.Header, strings.Join(s.Keywords, ", "))
}

// ShadowFix says how to resolve a shadow. authama never edits content outside
// its managed block, so the fix is the user's to make.
const ShadowFix = "delete or rename that block, or remove those lines from it " +
	"(authama does not edit content outside its managed block)"

// foreignBlock is one section of ssh_config outside the managed block.
type foreignBlock struct {
	line     int
	header   string
	patterns []string // nil means "every host" (top-level options, Match all)
	// match holds a Match block's host/originalhost pattern lists; every one
	// must match the alias. nil for a Host block or Match all.
	match    [][]string
	keywords map[string]bool
}

// findShadows reports each managed alias that a foreign block above the managed
// block applies to and that sets a keyword the managed block also sets for it.
//
// Only the preamble counts: content below the managed block is read after it,
// where the managed values have already won. A Match block is evaluated when
// its criteria are host and originalhost only (matchHostLists); any other is
// skipped, since a guess either way would be wrong. An Include is not followed. Both are rare in a
// config that `authama ssh import` read, which is where this shadowing comes from.
func findShadows(file, onDisk string, rendered map[string]string) []Shadow {
	lines := splitLines(onDisk)
	end := len(lines)
	for i, ln := range lines {
		if renderer.IsManagedFile(strings.TrimSpace(ln)) {
			end = i
			break
		}
	}
	foreign := parseBlocks(lines[:end])
	if len(foreign) == 0 {
		return nil
	}
	managed := managedKeywords(rendered)
	aliases := make([]string, 0, len(managed))
	for a := range managed {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)

	var out []Shadow
	for _, fb := range foreign {
		for _, alias := range aliases {
			if fb.patterns != nil && !hostMatches(fb.patterns, alias) {
				continue
			}
			if !matchApplies(fb.match, alias) {
				continue
			}
			var shared []string
			for kw, display := range managed[alias] {
				if fb.keywords[kw] {
					shared = append(shared, display)
				}
			}
			if len(shared) == 0 {
				continue // it applies, but overrides nothing the manifest sets
			}
			sort.Strings(shared)
			out = append(out, Shadow{Alias: alias, File: file, Line: fb.line, Header: fb.header, Keywords: shared})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

// parseBlocks splits config lines into blocks the way ssh scopes them: options
// before the first Host/Match apply to every host, a Host line opens a block
// for its patterns, and a Match line opens one that applies by its host and
// originalhost criteria, or, with any other criterion, one authama skips.
func parseBlocks(lines []string) []foreignBlock {
	var out []foreignBlock
	var cur *foreignBlock
	skip := false // inside a Match block we cannot evaluate
	for i, raw := range lines {
		kw, args := splitKeyword(raw)
		if kw == "" {
			continue
		}
		switch kw {
		case "host":
			out = append(out, foreignBlock{line: i + 1, header: strings.TrimSpace(raw),
				patterns: strings.Fields(args), keywords: map[string]bool{}})
			cur, skip = &out[len(out)-1], false
		case "match":
			if strings.EqualFold(strings.TrimSpace(args), "all") {
				out = append(out, foreignBlock{line: i + 1, header: strings.TrimSpace(raw), keywords: map[string]bool{}})
				cur, skip = &out[len(out)-1], false
			} else if lists, ok := matchHostLists(args); ok {
				out = append(out, foreignBlock{line: i + 1, header: strings.TrimSpace(raw), match: lists, keywords: map[string]bool{}})
				cur, skip = &out[len(out)-1], false
			} else {
				cur, skip = nil, true
			}
		default:
			if skip {
				continue
			}
			if cur == nil {
				out = append(out, foreignBlock{line: i + 1, header: "top level, applies to every host",
					keywords: map[string]bool{}})
				cur = &out[len(out)-1]
			}
			cur.keywords[kw] = true
		}
	}
	// A block that sets nothing cannot shadow anything.
	kept := out[:0]
	for _, b := range out {
		if len(b.keywords) > 0 {
			kept = append(kept, b)
		}
	}
	return kept
}

// matchHostLists reads a Match line made only of host and originalhost
// criteria, returning each one's comma-separated pattern list. Anything else
// (exec, user, a negated criterion, canonical, final) is not evaluated, and
// ok is false. Above the managed block no HostName has been set yet, so ssh
// matches both criteria against the alias (checked with ssh -G, 2026-11-14).
func matchHostLists(args string) ([][]string, bool) {
	f := strings.Fields(args)
	if len(f) == 0 || len(f)%2 != 0 {
		return nil, false
	}
	var lists [][]string
	for i := 0; i < len(f); i += 2 {
		switch strings.ToLower(f[i]) {
		case "host", "originalhost":
			lists = append(lists, strings.Split(strings.Trim(f[i+1], `"`), ","))
		default:
			return nil, false
		}
	}
	return lists, true
}

// matchApplies reports whether every pattern list of a Match block matches
// alias; a block with none (a Host block, Match all) applies.
func matchApplies(lists [][]string, alias string) bool {
	for _, l := range lists {
		if !hostMatches(l, alias) {
			return false
		}
	}
	return true
}

// managedKeywords maps each alias in the rendered managed files to the
// keywords its Host block sets (lower-cased key -> as written). Host * is not
// an alias and is left out.
func managedKeywords(rendered map[string]string) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, content := range rendered {
		var cur map[string]string
		for _, raw := range splitLines(content) {
			kw, args := splitKeyword(raw)
			switch kw {
			case "":
				continue
			case "host":
				cur = nil
				if alias := strings.TrimSpace(args); alias != "*" {
					cur = map[string]string{}
					out[alias] = cur
				}
			case "match":
				cur = nil
			default:
				if cur != nil {
					f := strings.Fields(raw)
					cur[kw] = strings.SplitN(f[0], "=", 2)[0]
				}
			}
		}
	}
	return out
}

// splitKeyword returns a config line's keyword, lower-cased, and the rest. ssh
// separates the two with whitespace or one '='. Blank and comment lines give "".
func splitKeyword(line string) (string, string) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", ""
	}
	i := strings.IndexAny(s, " \t=")
	if i < 0 {
		return strings.ToLower(s), ""
	}
	rest := strings.TrimLeft(s[i:], " \t")
	rest = strings.TrimPrefix(rest, "=")
	return strings.ToLower(s[:i]), strings.TrimSpace(rest)
}

// hostMatches applies a Host line's patterns to an alias as ssh does: the
// alias is lower-cased (ssh lower-cases the host it is given), a matching
// negated pattern rules the block out whatever else matches, and otherwise any
// matching pattern selects it.
func hostMatches(patterns []string, alias string) bool {
	host := strings.ToLower(alias)
	matched := false
	for _, p := range patterns {
		p = strings.Trim(p, `"`)
		if neg, ok := strings.CutPrefix(p, "!"); ok {
			if globMatch(neg, host) {
				return false
			}
			continue
		}
		if globMatch(p, host) {
			matched = true
		}
	}
	return matched
}

// globMatch is ssh's match_pattern: '*' matches any run (including none), '?'
// exactly one character, everything else itself. Unlike path.Match there are
// no character classes or escapes, and '*' crosses '/'.
func globMatch(pattern, s string) bool {
	if pattern == "" {
		return s == ""
	}
	switch pattern[0] {
	case '*':
		for i := 0; i <= len(s); i++ {
			if globMatch(pattern[1:], s[i:]) {
				return true
			}
		}
		return false
	case '?':
		return s != "" && globMatch(pattern[1:], s[1:])
	default:
		return s != "" && s[0] == pattern[0] && globMatch(pattern[1:], s[1:])
	}
}

func splitLines(text string) []string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines
}
