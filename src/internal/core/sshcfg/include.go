package sshcfg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/sangehq/authama/src/internal/core/renderer"
)

// MaxIncludeDepth is OpenSSH's SSH_MAX_INCLUDES: an Include nested more
// deeply than this is skipped, with a warning, so a cycle that slips past
// path identity cannot recurse forever.
const MaxIncludeDepth = 16

// Kind is one kind of Include warning.
type Kind string

const (
	WarnMissing    Kind = "missing"
	WarnCycle      Kind = "cycle"
	WarnDepth      Kind = "depth"
	WarnUnreadable Kind = "unreadable"
)

// Warning is a problem following an Include that does not stop the rest of
// the tree from being read: a path that is not there, a cycle, a depth
// limit, or a file that cannot be read.
type Warning struct {
	Kind    Kind   `json:"kind"`
	Path    string `json:"path,omitempty"`
	From    string `json:"from,omitempty"`
	Line    int    `json:"line,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Message string `json:"message"`
}

// Option is one keyword in a Host (or Match, or leading) block, attributed
// to the file and 1-based line it was written on.
type Option struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	File  string `json:"file"`
	Line  int    `json:"line"`
}

// Host is one Host (or leading-option) block as written in a single file,
// before later blocks fill gaps. Source is that file and the 1-based Host
// line (or the first option, for leading lines).
type Host struct {
	Alias      string   `json:"alias,omitempty"`
	Patterns   []string `json:"patterns,omitempty"`
	Options    []Option `json:"options,omitempty"`
	File       string   `json:"file"`
	Line       int      `json:"line"`
	EndLine    int      `json:"end_line"`
	Managed    bool     `json:"managed,omitempty"`
	InMatch    bool     `json:"in_match,omitempty"`
	HasInclude bool     `json:"has_include,omitempty"`
}

// Node is one file in the Include tree, or a path that could not be read.
type Node struct {
	Path          string  `json:"path"`
	Pattern       string  `json:"pattern,omitempty"`
	Line          int     `json:"line,omitempty"`
	Parent        string  `json:"parent,omitempty"`
	Managed       bool    `json:"managed,omitempty"`
	Missing       bool    `json:"missing,omitempty"`
	Cycle         bool    `json:"cycle,omitempty"`
	DepthExceeded bool    `json:"depth_exceeded,omitempty"`
	Unreadable    bool    `json:"unreadable,omitempty"`
	Includes      []*Node `json:"includes,omitempty"`
	Hosts         []Host  `json:"hosts,omitempty"`
}

// Tree is the Include graph of a user ssh config and every Host block
// reached through it, each attributed to its source file.
type Tree struct {
	Root     *Node     `json:"root"`
	Files    []string  `json:"files"`
	Dirs     []string  `json:"dirs"`
	Hosts    []Host    `json:"hosts"`
	Warnings []Warning `json:"warnings,omitempty"`
	SSHDir   string    `json:"ssh_dir,omitempty"`
}

// LoadUser reads ~/.ssh/config (sshDir/config) and every file it Includes.
func LoadUser(sshDir string) (*Tree, error) {
	return Load(filepath.Join(sshDir, "config"), sshDir)
}

// Load reads the ssh config at path and follows every Include, resolving
// relative paths against userSSHDir the way OpenSSH does for a user config.
// A missing root file is a tree with a warning, not an error.
func Load(path, userSSHDir string) (*Tree, error) {
	t := &Tree{SSHDir: userSSHDir}
	stack := map[string]bool{}
	t.Root = t.read(path, "", "", 0, 0, stack, userSSHDir)
	t.collect()
	return t, nil
}

func (t *Tree) read(path, parent, pattern string, line, depth int, stack map[string]bool, userSSHDir string) *Node {
	n := &Node{Path: path, Pattern: pattern, Line: line, Parent: parent}
	if depth > MaxIncludeDepth {
		n.DepthExceeded = true
		t.warn(WarnDepth, path, parent, line, pattern,
			fmt.Sprintf("Include %q at %s:%d is deeper than OpenSSH's limit of %d and was not read", pattern, display(parent, userSSHDir), line, MaxIncludeDepth))
		return n
	}
	fi, err := os.Lstat(path)
	if err != nil {
		n.Missing = true
		t.warn(WarnMissing, path, parent, line, pattern,
			fmt.Sprintf("Include %q at %s:%d names %s, which is not a file", patternOr(pattern, path), display(parent, userSSHDir), line, display(path, userSSHDir)))
		return n
	}
	_ = fi
	resolved := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		resolved = r
	}
	if stack[resolved] {
		n.Cycle = true
		t.warn(WarnCycle, path, parent, line, pattern,
			fmt.Sprintf("Include %q at %s:%d loops back to %s and was not read again", patternOr(pattern, path), display(parent, userSSHDir), line, display(path, userSSHDir)))
		return n
	}
	if !isRegular(path) {
		n.Missing = true
		t.warn(WarnMissing, path, parent, line, pattern,
			fmt.Sprintf("Include %q at %s:%d names %s, which is not a regular file", patternOr(pattern, path), display(parent, userSSHDir), line, display(path, userSSHDir)))
		return n
	}
	body, err := os.ReadFile(path)
	if err != nil {
		n.Unreadable = true
		t.warn(WarnUnreadable, path, parent, line, pattern,
			fmt.Sprintf("Include %q at %s:%d could not be read: %v", patternOr(pattern, path), display(parent, userSSHDir), line, err))
		return n
	}
	text := string(body)
	n.Managed = Managed(path, userSSHDir, text)
	stack[resolved] = true
	n.Hosts, n.Includes = t.parseFile(path, text, userSSHDir, depth, stack)
	delete(stack, resolved)
	return n
}

func (t *Tree) parseFile(file, text, userSSHDir string, depth int, stack map[string]bool) ([]Host, []*Node) {
	managedFile := renderer.IsManagedFile(text)
	inManaged := false
	inMatch := false
	var hosts []Host
	var includes []*Node
	var cur *Host
	finish := func() { cur = nil }

	for i, raw := range strings.Split(text, "\n") {
		lineNo := i + 1
		line := strings.TrimSpace(raw)
		if renderer.IsManagedHeaderLine(line) {
			inManaged = true
			finish()
			continue
		}
		if renderer.IsManagedEndLine(line) {
			inManaged = false
			finish()
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		keyword, value := splitKV(line)
		switch keyword {
		case "host":
			inMatch = false
			h := Host{
				Patterns: argvSplit(value),
				File:     file,
				Line:     lineNo,
				EndLine:  lineNo,
				Managed:  managedFile || inManaged,
			}
			for _, p := range h.Patterns {
				if !strings.ContainsAny(p, "*?!") {
					if h.Alias == "" {
						h.Alias = p
					}
				}
			}
			hosts = append(hosts, h)
			cur = &hosts[len(hosts)-1]
		case "match":
			inMatch = true
			finish()
		case "include":
			if cur != nil {
				cur.HasInclude = true
				cur.EndLine = lineNo
			}
			files, missing := ExpandLine(value, userSSHDir)
			for _, tok := range missing {
				// ExpandLine reports tokens that matched nothing; the child
				// node below is only added for a concrete path, so name the
				// unresolved token as the path for the warning.
				abs := expandTilde(tok)
				if !filepath.IsAbs(abs) {
					abs = filepath.Join(userSSHDir, abs)
				}
				child := &Node{Path: filepath.Clean(abs), Pattern: tok, Line: lineNo, Parent: file, Missing: true}
				includes = append(includes, child)
				t.warn(WarnMissing, child.Path, file, lineNo, tok,
					fmt.Sprintf("Include %q at %s:%d matched no file", tok, display(file, userSSHDir), lineNo))
			}
			for _, f := range files {
				includes = append(includes, t.read(f, file, valueToken(value, f, userSSHDir), lineNo, depth+1, stack, userSSHDir))
			}
		default:
			if cur != nil {
				cur.Options = append(cur.Options, Option{Key: keyword, Value: value, File: file, Line: lineNo})
				cur.EndLine = lineNo
			} else if !inMatch && len(hosts) == 0 && depth == 0 {
				// Leading options in the first file apply to every host.
				// Later included files inherit the includer's context, so
				// their leading lines are not a new global block.
				if len(hosts) == 0 {
					hosts = append(hosts, Host{Patterns: nil, File: file, Line: lineNo, EndLine: lineNo, Managed: managedFile || inManaged})
					cur = &hosts[0]
					cur.Options = append(cur.Options, Option{Key: keyword, Value: value, File: file, Line: lineNo})
				}
			} else if cur == nil && inMatch {
				// Options under Match without a current Host are not a Host
				// block; Includes under that Match are still followed above.
			}
		}
	}
	// Mark hosts whose Include was inside a Match by walking again would be
	// messy; parseFile records InMatch on Includes only via the children.
	// Stamp InMatch on a Host when it was opened while inMatch was already
	// true - but Host clears inMatch. Hosts under Match do not occur: a
	// Host line ends the Match. The flag belongs on included children.
	for _, c := range includes {
		if inMatchAt(text, c.Line) {
			for i := range c.Hosts {
				c.Hosts[i].InMatch = true
			}
		}
	}
	return hosts, includes
}

// inMatchAt reports whether the Include on 1-based line sits inside a Match
// block that has not been closed by a Host line or another Match.
func inMatchAt(text string, line int) bool {
	inMatch := false
	for i, raw := range strings.Split(text, "\n") {
		if i+1 > line {
			break
		}
		keyword, _ := splitKV(strings.TrimSpace(raw))
		switch keyword {
		case "match":
			inMatch = true
		case "host":
			inMatch = false
		}
	}
	return inMatch
}

func valueToken(value, matched, userSSHDir string) string {
	for _, tok := range strings.Fields(value) {
		found, _ := Expand(tok, userSSHDir)
		for _, f := range found {
			if f == matched {
				return tok
			}
		}
	}
	return value
}

func (t *Tree) warn(kind Kind, path, from string, line int, pattern, message string) {
	t.Warnings = append(t.Warnings, Warning{Kind: kind, Path: path, From: from, Line: line, Pattern: pattern, Message: message})
}

func (t *Tree) collect() {
	seenFile := map[string]bool{}
	seenDir := map[string]bool{}
	seenHost := map[string]bool{}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Path != "" && !n.Missing && !n.Cycle && !n.DepthExceeded && !n.Unreadable {
			if !seenFile[n.Path] {
				seenFile[n.Path] = true
				t.Files = append(t.Files, n.Path)
			}
			dir := filepath.Dir(n.Path)
			if !seenDir[dir] {
				seenDir[dir] = true
				t.Dirs = append(t.Dirs, dir)
			}
		}
		for _, h := range n.Hosts {
			key := fmt.Sprintf("%s:%d:%s", h.File, h.Line, h.Alias)
			if seenHost[key] {
				continue
			}
			seenHost[key] = true
			t.Hosts = append(t.Hosts, h)
		}
		for _, c := range n.Includes {
			walk(c)
		}
	}
	walk(t.Root)
	sort.Strings(t.Dirs)
}

// Managed reports whether authama may rewrite path: the user's ~/.ssh/config
// (whose managed region it already composes) or a file that opens with the
// managed header. Every other Included file is left alone; a rewrite of an
// IdentityFile in one of those is reported as a manual follow-up.
func Managed(path, sshDir, content string) bool {
	if renderer.IsManagedFile(content) {
		return true
	}
	if sshDir != "" && filepath.Clean(path) == filepath.Join(sshDir, "config") {
		return true
	}
	return false
}

// Display formats path for a person: ~/.ssh/<rel> when it is under sshDir,
// ~/rel when it is under the home that holds sshDir, otherwise the path
// itself with slashes.
func Display(path, sshDir string) string {
	return display(path, sshDir)
}

func display(path, sshDir string) string {
	if path == "" {
		return path
	}
	if sshDir != "" {
		if rel, err := filepath.Rel(sshDir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if rel == "." {
				return "~/.ssh"
			}
			return "~/.ssh/" + filepath.ToSlash(rel)
		}
		home := filepath.Dir(sshDir)
		if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "~/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func patternOr(pattern, path string) string {
	if pattern != "" {
		return pattern
	}
	return path
}

// Format is the human Include tree: each file, the Includes it names, and
// the Host blocks in it with their source line. Warnings follow, if any.
func (t *Tree) Format() string {
	var b strings.Builder
	if t.Root == nil {
		return "ssh config: no file to read"
	}
	formatNode(&b, t.Root, t.SSHDir, "")
	if len(t.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, w := range t.Warnings {
			fmt.Fprintf(&b, "  %s: %s\n", w.Kind, w.Message)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatNode(b *strings.Builder, n *Node, sshDir, indent string) {
	if n == nil {
		return
	}
	label := display(n.Path, sshDir)
	var tags []string
	if n.Managed {
		tags = append(tags, "managed")
	}
	if n.Missing {
		tags = append(tags, "missing")
	}
	if n.Cycle {
		tags = append(tags, "cycle")
	}
	if n.DepthExceeded {
		tags = append(tags, "depth")
	}
	if n.Unreadable {
		tags = append(tags, "unreadable")
	}
	if n.Line > 0 {
		fmt.Fprintf(b, "%s%s", indent, label)
		if n.Pattern != "" {
			fmt.Fprintf(b, "  (Include %s, line %d)", n.Pattern, n.Line)
		}
	} else {
		fmt.Fprintf(b, "%s%s", indent, label)
	}
	if len(tags) > 0 {
		fmt.Fprintf(b, "  [%s]", strings.Join(tags, ", "))
	}
	b.WriteByte('\n')
	for _, h := range n.Hosts {
		name := h.Alias
		if name == "" {
			if len(h.Patterns) == 0 {
				name = "(leading options)"
			} else {
				name = strings.Join(h.Patterns, " ")
			}
		} else if len(h.Patterns) > 1 {
			name = strings.Join(h.Patterns, " ")
		}
		fmt.Fprintf(b, "%s  Host %s  (line %d)\n", indent, name, h.Line)
	}
	for _, c := range n.Includes {
		formatNode(b, c, sshDir, indent+"  ")
	}
}

// MarshalJSON is the --json shape: the tree plus the collected lists.
func (t *Tree) JSON() ([]byte, error) {
	return json.MarshalIndent(t, "", "  ")
}

// splitKV splits a config line into a lowercased keyword and the trimmed rest,
// the way OpenSSH's strdelim allows: `HostName=x`, `HostName = x` and
// `HostName x` all mean the same thing.
func splitKV(line string) (string, string) {
	i := strings.IndexFunc(line, func(r rune) bool { return r == '=' || unicode.IsSpace(r) })
	if i < 0 {
		return strings.ToLower(line), ""
	}
	key := strings.ToLower(strings.TrimSpace(line[:i]))
	rest := strings.TrimSpace(line[i:])
	rest = strings.TrimPrefix(rest, "=")
	return key, strings.TrimSpace(rest)
}

func argvSplit(value string) []string {
	return strings.Fields(value)
}
