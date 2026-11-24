package configsvc

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/core/sshcfg"
	"github.com/sangehq/authama/src/internal/util/fs"
)

// Collision is a hand-written Host block, outside the managed block, that
// names an alias the manifest also renders. ssh then has two definitions of
// one host: the first it reads wins for every option both set, and the other's
// remaining options still apply. It is what `authama ssh import` used to leave
// behind (review R-BUG-09), and what a hand edit can still add.
type Collision struct {
	Alias  string
	File   string
	Line   int // 1-based, of the Host line
	Header string
}

func (c Collision) String() string {
	return fmt.Sprintf("%s is also defined by %s:%d (%s), outside authama's block", c.Alias, c.File, c.Line, c.Header)
}

// CollisionFix is how to resolve a collision.
const CollisionFix = "run `authama ssh config adopt --dry-run` to see which of those blocks the manifest " +
	"already reproduces, then `authama ssh config adopt` to move them out of your config " +
	"(a block it cannot reproduce is listed with the line that differs, for you to resolve)"

// userBlock is one Host block of the root config outside the managed block.
type userBlock struct {
	start, end int // 0-based: the Host line, and one past its last option line
	header     string
	patterns   []string
	opts       []renderer.OptionLine
	include    bool
	above      bool // in the preamble, read before the managed block
}

// userBlocks splits the root config's foreign content into Host blocks. Match
// blocks and top-level options are not Host blocks and are not returned, but
// they end the block before them.
func userBlocks(lines []string) []userBlock {
	var out []userBlock
	var cur *userBlock
	inManaged, seenManaged := false, false
	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		if renderer.IsManagedHeaderLine(t) {
			inManaged, seenManaged, cur = true, true, nil
			continue
		}
		if renderer.IsManagedEndLine(t) {
			inManaged = false
			continue
		}
		if inManaged {
			continue
		}
		kw, args := splitKeyword(raw)
		switch kw {
		case "":
			continue
		case "host":
			out = append(out, userBlock{start: i, end: i + 1, header: t,
				patterns: strings.Fields(args), above: !seenManaged})
			cur = &out[len(out)-1]
		case "match":
			cur = nil
		case "include":
			if cur != nil {
				cur.include = true
				cur.end = i + 1
			}
		default:
			if cur != nil {
				f := strings.Fields(raw)
				cur.opts = append(cur.opts, renderer.OptionLine{Key: strings.SplitN(f[0], "=", 2)[0], Value: args})
				cur.end = i + 1
			}
		}
	}
	return out
}

// findCollisions reports every foreign Host block in the root config that
// names a managed alias. Content in files the user's own lines Include is not
// read, as findShadows does not read it.
func findCollisions(file, onDisk string, rendered map[string]string) []Collision {
	managed := managedKeywords(rendered)
	var out []Collision
	for _, b := range userBlocks(splitLines(onDisk)) {
		for _, p := range b.patterns {
			p = strings.Trim(p, `"`)
			for alias := range managed {
				if strings.EqualFold(p, alias) {
					out = append(out, Collision{Alias: alias, File: file, Line: b.start + 1, Header: b.header})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Alias < out[j].Alias
	})
	return out
}

// AdoptResult is what `authama ssh config adopt` did, or would do.
type AdoptResult struct {
	DryRun bool
	Moved  []Collision
	// Kept are the colliding blocks left where they are, each with what the
	// rendered host does not reproduce.
	Kept   []string
	Diff   string
	Backup string
}

// Format is the human report.
func (r *AdoptResult) Format() string {
	var b strings.Builder
	verb := "moved"
	if r.DryRun {
		verb = "would move"
	}
	if len(r.Moved) == 0 && len(r.Kept) == 0 {
		return "config adopt: no hand-written block defines a managed host ✓"
	}
	for _, c := range r.Moved {
		fmt.Fprintf(&b, "%s %s:%d (%s): the manifest renders %s the same way\n", verb, c.File, c.Line, c.Header, c.Alias)
	}
	for _, k := range r.Kept {
		fmt.Fprintf(&b, "kept %s\n", k)
	}
	if r.Backup != "" {
		fmt.Fprintf(&b, "the config before this is in %s\n", r.Backup)
	}
	if r.DryRun && r.Diff != "" {
		b.WriteString(r.Diff)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Adopt takes out of ~/.ssh/config each hand-written Host block that defines
// a managed alias and that the managed block already reproduces line for line,
// so ssh reads one definition of the host and resolves it exactly as before.
// It is how a home onboarded before import adopted its blocks is brought in
// line, and it is idempotent: a second run finds nothing.
//
// A block is kept, and reported with the reason, when it also names a host the
// manifest does not, holds an Include, sets something the render does not
// reproduce, or sits above the managed block with a later foreign block that
// would then supply a value in its place. Nothing else in the file changes, and
// the managed block does not move.
//
// backupDir receives a copy of the config before it is written; with dryRun
// nothing is written and Diff shows the change.
func (s *Service) Adopt(dryRun bool, backupDir string) (*AdoptResult, error) {
	rendered, err := s.Rendered()
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(s.sshDir, renderer.RootConfig)
	res := &AdoptResult{DryRun: dryRun}
	current, ok := readFile(dest)
	if !ok {
		return res, nil
	}
	lines := splitLines(current)
	blocks := userBlocks(lines)
	managed := managedKeywords(rendered)
	global := renderedGlobalKeywords(rendered)
	var drop []renderer.LineRange
	for bi, b := range blocks {
		var named []string
		for _, p := range b.patterns {
			if alias, ok := managedAlias(managed, strings.Trim(p, `"`)); ok {
				named = append(named, alias)
			}
		}
		if len(named) == 0 {
			continue
		}
		where := fmt.Sprintf("%s:%d (%s)", dest, b.start+1, b.header)
		why := s.adoptable(b, named, blocks[bi+1:], managed, global)
		if why != "" {
			res.Kept = append(res.Kept, where+": "+why)
			continue
		}
		drop = append(drop, renderer.LineRange{Start: b.start, End: b.end})
		for _, a := range named {
			res.Moved = append(res.Moved, Collision{Alias: a, File: dest, Line: b.start + 1, Header: b.header})
		}
	}
	if len(drop) == 0 {
		return res, nil
	}
	next := renderer.StripLines(current, drop, -1)
	if dryRun {
		res.Diff = unifiedDiff(current, next, renderer.RootConfig)
		return res, nil
	}
	if err := os.MkdirAll(backupDir, dirMode); err != nil {
		return nil, err
	}
	saved := filepath.Join(backupDir, "config")
	if err := fs.WriteTextAtomic(saved, current, configMode); err != nil {
		return nil, fmt.Errorf("could not back the config up, so it was not edited: %w", err)
	}
	res.Backup = saved
	if err := WriteConfigFile(dest, next); err != nil {
		return nil, err
	}
	return res, nil
}

func managedAlias(managed map[string]map[string]string, p string) (string, bool) {
	for alias := range managed {
		if strings.EqualFold(alias, p) {
			return alias, true
		}
	}
	return "", false
}

// adoptable says why block b cannot go, or "" when it can.
func (s *Service) adoptable(b userBlock, named []string, later []userBlock,
	managed map[string]map[string]string, global map[string]bool) string {
	if len(named) != len(b.patterns) {
		return "its Host line also names something the manifest does not render"
	}
	if b.include {
		return "it holds an Include, which a managed host cannot hold"
	}
	for _, alias := range named {
		h, profile, ok := s.manifestHost(alias)
		if !ok {
			return alias + " is rendered from defaults.git, not from a manifest host; remove the block by hand if it is not wanted"
		}
		for _, o := range b.opts {
			key := strings.ToLower(o.Key)
			// Below the managed block, a single value ssh already has from the
			// managed host or authama's Host * is never read.
			if !b.above && !renderer.MultiValued(key) && (managed[alias][key] != "" || global[key]) {
				continue
			}
			if ok, why := renderer.Represents(s.manifest, profile, h, o, s.sameKeyAs(profile, h)); !ok {
				return why
			}
		}
		if b.above {
			// Removing a block above the managed one lets any foreign block
			// between it and the managed block supply its values first.
			for _, l := range later {
				if !l.above || !hostMatches(l.patterns, alias) {
					continue
				}
				for _, o := range b.opts {
					for _, lo := range l.opts {
						if strings.EqualFold(o.Key, lo.Key) {
							return fmt.Sprintf("line %d (%s) also sets %s for %s and would apply in its place", l.start+1, l.header, o.Key, alias)
						}
					}
				}
			}
		}
	}
	return ""
}

func (s *Service) manifestHost(alias string) (manifest.Host, string, bool) {
	for _, pname := range s.manifest.ProfileNames() {
		for _, h := range s.manifest.Profiles[pname].Hosts {
			if strings.EqualFold(h.Alias, alias) {
				return h, pname, true
			}
		}
	}
	return manifest.Host{}, "", false
}

// sameKeyAs reports whether a path holds the key the host's rendered
// IdentityFile names: the same public key, or the same private key bytes. An
// import copies the key it adopts, so the user's block points at the original
// and the manifest at the copy.
func (s *Service) sameKeyAs(profile string, h manifest.Host) func(string) bool {
	return func(path string) bool {
		kname, err := s.manifest.ResolvedKeyName(profile, h)
		if err != nil {
			return false
		}
		ours := s.manifest.KeyPath(s.sshDir, manifest.KeyRef{Profile: profile, KeyName: kname})
		return sameKeyFile(path, ours)
	}
}

func sameKeyFile(a, b string) bool {
	pubA, errA := os.ReadFile(strings.TrimSuffix(a, ".pub") + ".pub")
	pubB, errB := os.ReadFile(strings.TrimSuffix(b, ".pub") + ".pub")
	if errA == nil && errB == nil {
		fa, fb := strings.Fields(string(pubA)), strings.Fields(string(pubB))
		return len(fa) >= 2 && len(fb) >= 2 && fa[0] == fb[0] && fa[1] == fb[1]
	}
	privA, errA := os.ReadFile(a)
	privB, errB := os.ReadFile(b)
	return errA == nil && errB == nil && len(privA) > 0 && bytes.Equal(privA, privB)
}

// renderedGlobalKeywords are the keywords authama's own Host * block sets.
func renderedGlobalKeywords(rendered map[string]string) map[string]bool {
	out := map[string]bool{}
	in := false
	for _, raw := range splitLines(rendered[renderer.RootConfig]) {
		kw, args := splitKeyword(raw)
		switch kw {
		case "":
		case "host":
			in = strings.TrimSpace(args) == "*"
		case "match":
			in = false
		default:
			if in {
				out[kw] = true
			}
		}
	}
	return out
}

// collisionsFromTree reports Host blocks in files ~/.ssh/config Includes that
// authama does not manage, when they name a managed alias. The blocks stay
// where they are: adopt only edits ~/.ssh/config.
func collisionsFromTree(tree *sshcfg.Tree, sshDir string, rendered map[string]string) []Collision {
	if tree == nil {
		return nil
	}
	managed := managedKeywords(rendered)
	var out []Collision
	for _, h := range tree.Hosts {
		if h.Managed || h.Alias == "" {
			continue
		}
		if filepath.Clean(h.File) == filepath.Join(sshDir, renderer.RootConfig) {
			continue // findCollisions already covers the root file's foreign blocks
		}
		if _, ok := managed[h.Alias]; !ok {
			continue
		}
		header := "Host " + strings.Join(h.Patterns, " ")
		out = append(out, Collision{Alias: h.Alias, File: sshcfg.Display(h.File, sshDir), Line: h.Line, Header: header})
	}
	return out
}

func mergeCollisions(a, b []Collision) []Collision {
	seen := map[string]bool{}
	var out []Collision
	for _, c := range append(append([]Collision{}, a...), b...) {
		key := c.File + ":" + fmt.Sprint(c.Line) + ":" + c.Alias
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}
