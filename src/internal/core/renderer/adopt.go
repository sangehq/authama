package renderer

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// OptionLine is one option of a hand-written Host block, as written: Key is
// the keyword in any case, Value the rest of the line, quotes included.
type OptionLine struct{ Key, Value string }

// multiValued are the keywords ssh accumulates instead of keeping the first
// value it reads (ssh_config(5)): every IdentityFile line names another key to
// try, every LocalForward another forward.
var multiValued = map[string]bool{
	"identityfile": true, "certificatefile": true, "localforward": true,
	"remoteforward": true, "dynamicforward": true, "sendenv": true, "setenv": true,
}

// MultiValued reports whether ssh adds up a keyword's values rather than
// keeping the first one. A later line for such a keyword is never dead.
func MultiValued(key string) bool { return multiValued[strings.ToLower(key)] }

// Represents reports whether the Host block authama renders for host, of
// profile, carries what opt sets, so that a hand-written line for the same
// alias can go and ssh does the same thing without it. This is the rule
// `authama ssh import` and `authama ssh config adopt` both apply before they
// remove a hand-written block: a line the render does not reproduce keeps the
// block where it is.
//
// sameKey reports whether a path names the key the rendered IdentityFile
// points at (an adopted copy of it, say); nil compares paths only. When the
// line is not carried, why says what removing it would lose.
func Represents(m *manifest.Manifest, profile string, h manifest.Host, opt OptionLine, sameKey func(path string) bool) (ok bool, why string) {
	key := strings.ToLower(strings.TrimSpace(opt.Key))
	val := strings.TrimSpace(opt.Value)
	arg := unquoteArg(val)
	lost := func() (bool, string) {
		return false, opt.Key + " " + val + " is not in what authama renders for " + h.Alias
	}
	switch key {
	case "hostname":
		if strings.EqualFold(arg, h.Hostname) {
			return true, ""
		}
		return lost()
	case "user":
		if arg == h.User {
			return true, ""
		}
		return lost()
	case "port":
		n, err := strconv.Atoi(arg)
		want := h.Port
		if want == 0 {
			want = 22
		}
		if err == nil && n == want {
			return true, ""
		}
		return lost()
	case "identityfile":
		kname, err := m.ResolvedKeyName(profile, h)
		if err != nil {
			return lost()
		}
		file, _ := identityFor(m, profile, kname)
		if sameIdentityPath(arg, file) || (sameKey != nil && sameKey(expandTilde(arg))) {
			return true, ""
		}
		return lost()
	case "userknownhostsfile":
		if sameIdentityPath(arg, m.KnownHostsFile()) {
			return true, ""
		}
		return lost()
	case "identitiesonly":
		if strings.EqualFold(arg, identitiesOnly(h.RawOptions)) {
			return true, ""
		}
		return lost()
	case "proxycommand":
		// The same proxy, however it was spelled: cloudflared from any path,
		// none, or the same command.
		p, err := manifest.ProxyFromCommand(val)
		if err == nil && h.Proxy != nil && p.Kind == h.Proxy.Kind && slices.Equal(p.Command, h.Proxy.Command) {
			return true, ""
		}
		return lost()
	}
	if manifest.IsDangerousOption(key) {
		return false, opt.Key + " is a directive authama never renders"
	}
	for _, k := range h.RawOptions.Keys() {
		if strings.EqualFold(k, key) && strings.TrimSpace(h.RawOptions.Get(k)) == val {
			return true, ""
		}
	}
	return lost()
}

// unquoteArg is a single-argument value with surrounding quotes removed, the
// way ssh reads HostName "x".
func unquoteArg(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// sameIdentityPath compares two paths as ssh would open them: ~ expanded.
func sameIdentityPath(a, b string) bool {
	return filepath.Clean(expandTilde(a)) == filepath.Clean(expandTilde(b))
}

func expandTilde(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return filepath.FromSlash(p)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.FromSlash(p)
	}
	return filepath.Join(home, filepath.FromSlash(strings.TrimPrefix(p[1:], "/")))
}

// LineRange is a half-open range of 0-based line indexes, [Start, End).
type LineRange struct{ Start, End int }

// StripLines returns content without the lines in drop, and nothing else
// changed: a blank line left on both sides of a removed range is kept once.
//
// With at >= 0 and no managed block in content, the managed markers go where
// line at was, so ComposeRootConfig renders the block in that place instead of
// appending it at the end. Import uses this to put the managed hosts where the
// hand-written blocks they replace were, so whatever the user wrote after them
// (a Host * of defaults, say) is still read after them.
func StripLines(content string, drop []LineRange, at int) string {
	lines := strings.Split(content, "\n")
	gone := make([]bool, len(lines))
	for _, r := range drop {
		for i := max(r.Start, 0); i < r.End && i < len(lines); i++ {
			gone[i] = true
		}
	}
	insert := at >= 0 && !hasManagedBlock(lines)
	var out []string
	for i, ln := range lines {
		if insert && i == at {
			out = append(out, ManagedHeader, ManagedEnd)
		}
		if gone[i] {
			continue
		}
		// A removed range between two blank lines would leave two in a row.
		if strings.TrimSpace(ln) == "" && i > 0 && gone[i-1] && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			continue
		}
		out = append(out, ln)
	}
	if insert && at >= len(lines) {
		out = append(out, ManagedHeader, ManagedEnd)
	}
	return strings.Join(out, "\n")
}

func hasManagedBlock(lines []string) bool {
	for _, ln := range lines {
		if isManagedHeader(strings.TrimSpace(ln)) {
			return true
		}
	}
	return false
}
