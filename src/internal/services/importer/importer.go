// Package importer onboards an existing ~/.ssh into the manifest + inventory.
// It parses an ssh config (following relative
// Include directives) into profiles + hosts, fingerprints any private keys it can
// find into the inventory, and adopts non-canonical keys into the profiles/ layout.
// Profile assignment derives from an IdentityFile under profiles/<profile>/ or
// profiles/<category>/<profile>/ (which also carries the category over), else
// "imported".
package importer

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/key"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// kv is one ordered raw_option carried through from the source config.
type kv struct{ Key, Val string }

// ParsedHost is one host block parsed from an ssh config.
type ParsedHost struct {
	Alias        string
	Hostname     string
	User         string
	Port         int
	IdentityFile string // "" if none
	Profile      string
	Category     string // "" unless the IdentityFile sat under profiles/<category>/<profile>/
	Extra        []kv
	// Aliases are the other names on its own Host line (Host web1 web1-cf),
	// when no block of their own names them first.
	Aliases []string
	// ProxyCommand is its own block's ProxyCommand, as written; "" if none.
	ProxyCommand string
	// Section is the comment right above its own Host line, and Comment the
	// one right under it, as authama renders a host's section and comment.
	Section, Comment string
}

// ImportResult summarizes an import run.
type ImportResult struct {
	Profiles  map[string]int // profile -> host count (counts all parsed hosts)
	KeysFound int
	Adopted   int
	DryRun    bool
	// Skipped names each host left out because the manifest would refuse it
	// (e.g. a hostname ssh itself refuses), with the reason.
	Skipped []string
	// Sources are the config files read, in order, and Root the folder
	// relative Includes and ~/.ssh/ identity paths resolved against.
	Sources []string
	Root    string
	// LeftInConfig are the aliases imported from hand-written Host blocks
	// that stayed in the source config.
	//
	// Deprecated: import now moves each block it imports out of the config
	// ssh reads (Moved), and leaves out a host whose block it cannot move
	// (Kept), so no imported alias is left defined twice and this is always
	// empty. Kept for callers of the struct until at least v1.0.0.
	LeftInConfig []string
	// NotImported are the Match blocks and wildcard-only Host blocks, as
	// written: authama manages plain Host blocks, so these stay and apply.
	NotImported []string
	// Moved are the aliases whose hand-written Host blocks import took out of
	// the config ssh reads, now that the manifest holds them; MovedFrom the
	// files it edited. The originals are kept in Backup.
	Moved     []string
	MovedFrom []string
	// Kept are the hosts left in the user's config and not imported, each
	// with why: a block import cannot move without changing what ssh does.
	Kept []string
	// Backup is where the files import edited were copied first.
	Backup string
}

// Format renders the human-readable import summary (mirrors ImportResult.format).
func (r ImportResult) Format() string {
	head := "import:"
	if r.DryRun {
		head = "import (dry-run):"
	}
	lines := []string{head}
	if len(r.Sources) > 1 {
		lines = append(lines, "  read "+strconv.Itoa(len(r.Sources))+" config files under "+r.Root)
	}
	names := make([]string, 0, len(r.Profiles))
	for p := range r.Profiles {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		lines = append(lines, "  profile "+p+": "+strconv.Itoa(r.Profiles[p])+" host(s)")
	}
	lines = append(lines, "  keys fingerprinted into inventory: "+strconv.Itoa(r.KeysFound))
	if r.Adopted > 0 {
		lines = append(lines, "  keys adopted into profiles/ layout: "+strconv.Itoa(r.Adopted))
	}
	for _, s := range r.Skipped {
		lines = append(lines, "  skipped "+s)
	}
	if len(r.Moved) > 0 {
		verb := "moved"
		if r.DryRun {
			verb = "would move"
		}
		line := "  " + verb + " into the manifest, out of " + strings.Join(r.MovedFrom, ", ") + ": " + strings.Join(r.Moved, ", ")
		if r.Backup != "" {
			line += " (the original is in " + r.Backup + ")"
		}
		lines = append(lines, line,
			"    authama renders these hosts now; ssh reads one definition of each")
	}
	for _, k := range r.Kept {
		lines = append(lines, "  kept in your config, not imported: "+k)
	}
	if len(r.LeftInConfig) > 0 {
		lines = append(lines, "  the hand-written Host blocks for "+strings.Join(r.LeftInConfig, ", ")+" stay in your config")
	}
	for _, n := range r.NotImported {
		lines = append(lines, "  not imported: "+n+" (authama manages plain Host blocks; it stays in your config and still applies)")
	}
	return strings.Join(lines, "\n")
}

var providerByHost = map[string]string{
	"github.com": "github", "gitlab.com": "gitlab", "bitbucket.org": "bitbucket",
}

func inferProvider(hostname string) *string {
	if p, ok := providerByHost[strings.ToLower(hostname)]; ok {
		return &p
	}
	return nil
}

// block is one section of an ssh config: a Host line's patterns and the options
// under it, in file order.
type block struct {
	patterns []string // as written, "!" negations included
	all      bool     // the lines before the first Host line: they apply to every host
	opts     []kv
	// section is the comment line right above the Host line, comment the
	// first one right under it, before any option.
	section, comment string

	// Where the block is, so import can take it out of the file once the
	// manifest holds it. file is the path the block was reached by (a link
	// not resolved, so a ~/.ssh/config that links into a dotfiles checkout
	// still reads as ~/.ssh/config). start is the Host line, end one past the
	// last option line, both 0-based. rootLine is where in the first file read
	// the block is: its own Host line there, or the Include line that pulled
	// its file in.
	file       string
	start, end int
	rootLine   int
	// managed is a block authama rendered (inside the managed markers, or in a
	// file carrying the managed header): a render replaces it, so it is never
	// edited and never counts as the user's own definition.
	managed bool
	// include is set when an Include sits inside the Host block. ssh scopes the
	// included file to the block, which no manifest host can say.
	include bool
	// first is set on the blocks read from the first source file and what it
	// Includes: rootLine is a line of that file.
	first bool
}

// matches reports whether this block applies to alias, as ssh's
// match_pattern_list decides it: some pattern matches and no negated one does.
func (b *block) matches(alias string) bool {
	if b.all {
		return true
	}
	hit := false
	for _, p := range b.patterns {
		if neg := strings.HasPrefix(p, "!"); neg {
			if globMatch(p[1:], alias) {
				return false
			}
		} else if globMatch(p, alias) {
			hit = true
		}
	}
	return hit
}

// names reports whether alias is written literally in this block's Host line,
// which makes it that host's own block rather than one it merely matches.
func (b *block) names(alias string) bool {
	for _, p := range b.patterns {
		if p == alias {
			return true
		}
	}
	return false
}

// parseSSHConfig parses ssh config text into hosts, following relative Include
// files.
//
// It resolves each host the way ssh does rather than block by block: every
// block whose patterns match the alias contributes, in file order, and the
// first value obtained for an option wins. So `Host *` and pattern blocks
// supply what a host leaves unset (before any $USER/22 fallback), a block
// above the host's own beats it, and a repeated alias only fills gaps. Raw
// options are carried only from the block that first names the host - a
// wildcard's options belong in the manifest's global options, not copied
// into every host. Match blocks are never applied: their criteria cannot be
// evaluated at import time.
func parseSSHConfig(text, baseDir string, seen map[string]bool) []*ParsedHost {
	blocks, aliases := collectBlocks(text, "", baseDir, seen, true, -1)
	hosts := make([]*ParsedHost, 0, len(aliases))
	for _, alias := range aliases {
		hosts = append(hosts, resolveHost(alias, blocks))
	}
	return hosts
}

// collectBlocks splits config text into blocks, inlining Include files in place,
// and lists each literal (non-pattern) alias once, in first-seen order.
//
// file is the path text was read from and rootLine the line of the first file
// read that leads here (-1 when text is that file).
func collectBlocks(text, file, baseDir string, seen map[string]bool, top bool, rootLine int) ([]*block, []string) {
	var blocks []*block
	var aliases []string
	known := map[string]bool{}
	// Options before the first Host line apply to every host - but only in the
	// file ssh reads first. An included file's leading lines inherit the
	// includer's context, which (as before) is treated as no context at all.
	var current *block
	if top {
		current = &block{all: true, file: file, start: -1, end: -1, rootLine: rootLine}
		blocks = append(blocks, current)
	}
	managedFile := renderer.IsManagedFile(text)
	inManaged := false
	pending := "" // the comment line just read, which a Host line may take as its section
	for i, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if renderer.IsManagedHeaderLine(line) {
			inManaged = true
		} else if renderer.IsManagedEndLine(line) {
			inManaged = false
		}
		if line == "" {
			pending = ""
			continue
		}
		if strings.HasPrefix(line, "#") {
			text := strings.TrimSpace(strings.TrimLeft(line, "#"))
			switch {
			case isBanner(line):
				pending = ""
			case current != nil && !current.all && len(current.opts) == 0 && current.comment == "" && pending == "":
				current.comment = text
			default:
				pending = text
			}
			continue
		}
		section := pending
		pending = ""
		at := rootLine
		if at < 0 {
			at = i
		}
		keyword, value := splitKV(line)
		// A comment line between a block's options is that block's comment
		// (an option typed in above it moves it down), not a section.
		if section != "" && keyword != "host" && keyword != "match" && current != nil && !current.all && current.comment == "" {
			current.comment, section = section, ""
		}
		switch {
		case keyword == "host":
			current = &block{patterns: argvSplit(value), file: file, start: i, end: i + 1,
				rootLine: at, managed: managedFile || inManaged, section: section}
			blocks = append(blocks, current)
			for _, p := range current.patterns {
				if strings.ContainsAny(p, "*?!") || known[p] {
					continue
				}
				known[p] = true
				aliases = append(aliases, p)
			}
		case keyword == "match":
			current = nil
		case keyword == "include" && baseDir != "":
			if current != nil && !current.all {
				current.include = true
			}
			incBlocks, incAliases := parseIncludes(value, baseDir, seen, at)
			blocks = append(blocks, incBlocks...)
			for _, a := range incAliases {
				if !known[a] {
					known[a] = true
					aliases = append(aliases, a)
				}
			}
			current = nil
		default:
			if current != nil {
				current.opts = append(current.opts, kv{keyword, value})
				if !current.all {
					current.end = i + 1
				}
			}
		}
	}
	return blocks, aliases
}

// resolveHost applies every block matching alias, in order, first value wins.
func resolveHost(alias string, blocks []*block) *ParsedHost {
	h := &ParsedHost{Alias: alias, Profile: "imported"}
	set := map[string]bool{}
	own := true // the first block naming alias is its own; later ones only fill gaps
	for _, b := range blocks {
		if !b.matches(alias) {
			continue
		}
		carry := own && b.names(alias)
		if carry {
			h.Section, h.Comment = b.section, b.comment
		}
		for _, o := range b.opts {
			applyOption(h, set, o.Key, o.Val, carry)
		}
		if carry {
			own = false
		}
	}
	if !set["port"] {
		h.Port = 22
	}
	if h.IdentityFile != "" {
		h.Profile, h.Category = profileFromIdentity(h.IdentityFile)
	}
	return h
}

// splitKV splits a config line into a lowercased keyword and the trimmed rest.
// The keyword ends at whitespace or '=', and one '=' (with whitespace either
// side) may separate it from the value, as OpenSSH's strdelim allows - so
// `HostName=x`, `HostName = x` and `HostName x` all mean the same thing.
func splitKV(line string) (string, string) {
	i := strings.IndexFunc(line, func(r rune) bool { return r == '=' || unicode.IsSpace(r) })
	if i < 0 {
		return strings.ToLower(line), ""
	}
	rest := strings.TrimLeftFunc(line[i:], unicode.IsSpace)
	rest = strings.TrimPrefix(rest, "=")
	return strings.ToLower(line[:i]), strings.TrimSpace(rest)
}

// applyOption records one option on h unless an earlier one already set it:
// ssh keeps the first value it obtains for each option. carry says whether
// this is the host's own block, the only place raw options are taken from.
func applyOption(h *ParsedHost, set map[string]bool, keyword, value string, carry bool) {
	switch keyword {
	case "hostname", "user", "port", "identityfile":
		if set[keyword] {
			return
		}
		// These take one argument, which ssh unquotes: `IdentityFile "~/my key"`
		// names a path with a space, not one beginning with a quote.
		arg := firstArg(value)
		switch keyword {
		case "hostname":
			h.Hostname = arg
		case "user":
			h.User = arg
		case "port":
			n, err := strconv.Atoi(arg)
			if err != nil {
				return // unusable; leave it unset so a later value can apply
			}
			h.Port = n
		case "identityfile":
			h.IdentityFile = arg
		}
		set[keyword] = true
	case "proxycommand":
		// Its own block's ProxyCommand becomes the host's proxy (none,
		// cloudflared, or a command authama can quote); build leaves out a
		// host whose command it cannot hold.
		if carry && !set[keyword] {
			h.ProxyCommand = value
			set[keyword] = true
		}
	default:
		if !carry || manifest.IsDangerousOption(keyword) {
			return // drop command/config-executing directives
		}
		// Carried verbatim, quotes included: it is rendered back into an ssh
		// config, where ssh will unquote it itself.
		h.Extra = append(h.Extra, kv{keyword, value})
	}
}

// firstArg is the first argument of an option value, unquoted.
func firstArg(value string) string {
	if args := argvSplit(value); len(args) > 0 {
		return args[0]
	}
	return ""
}

// argvSplit splits s into arguments as OpenSSH's argv_split does: spaces and
// tabs separate, single or double quotes group (and are removed), and a
// backslash escapes a quote, a backslash, or (outside quotes) a space. Any
// other backslash is literal, which keeps a Windows path like `~\.ssh\id`
// intact. ssh rejects an unterminated quote; here the partial argument is
// kept, since import should salvage what it can rather than fail outright.
func argvSplit(s string) []string {
	var args []string
	for i := 0; i < len(s); {
		if s[i] == ' ' || s[i] == '\t' {
			i++
			continue
		}
		var arg strings.Builder
		var quote byte
		for ; i < len(s); i++ {
			c := s[i]
			if c == '\\' && i+1 < len(s) && (s[i+1] == '\'' || s[i+1] == '"' || s[i+1] == '\\' ||
				(quote == 0 && s[i+1] == ' ')) {
				i++
				arg.WriteByte(s[i])
				continue
			}
			if quote == 0 && (c == ' ' || c == '\t') {
				break // unquoted whitespace ends the argument
			}
			switch {
			case quote == 0 && (c == '"' || c == '\''):
				quote = c
			case quote != 0 && c == quote:
				quote = 0
			default:
				arg.WriteByte(c)
			}
		}
		args = append(args, arg.String())
	}
	return args
}

// globMatch matches s against an ssh pattern, where '*' is any run of
// characters and '?' any one. Case-insensitive, as ssh lowercases the host
// it is matching.
func globMatch(pattern, s string) bool {
	p, t := []rune(strings.ToLower(pattern)), []rune(strings.ToLower(s))
	i, j, star, mark := 0, 0, -1, 0
	for j < len(t) {
		switch {
		case i < len(p) && (p[i] == '?' || p[i] == t[j]):
			i++
			j++
		case i < len(p) && p[i] == '*':
			star, mark = i, j // remember the star; try matching nothing first
			i++
		case star >= 0:
			mark++ // backtrack: let the last star swallow one more character
			i, j = star+1, mark
		default:
			return false
		}
	}
	for i < len(p) && p[i] == '*' {
		i++
	}
	return i == len(p)
}

// profileFromIdentity reads the profile (and category, if any) off a key path
// under profiles/: profiles/<profile>/<key> or
// profiles/<category>/<profile>/<key>. A flat profile's rotation archive,
// profiles/<profile>/old/<key>, is three segments deep too and is not a
// category.
func profileFromIdentity(identityFile string) (profile, category string) {
	return profileFromIdentityIn(identityFile, manifest.DefaultKeyRoot)
}

// profileFromIdentityIn is profileFromIdentity for a path under any of the
// key roots given: <root>/<profile>/<key> or <root>/<category>/<profile>/<key>.
func profileFromIdentityIn(identityFile string, roots ...string) (profile, category string) {
	parts := strings.Split(filepath.ToSlash(identityFile), "/")
	for i, p := range parts {
		if !slices.Contains(roots, p) || i+1 >= len(parts) {
			continue
		}
		rest := parts[i+1:]
		if len(rest) == 3 && rest[1] != "old" {
			return rest[1], rest[0]
		}
		return rest[0], ""
	}
	return "imported", ""
}

// parseIncludes returns the blocks and aliases of every file an Include names,
// in order, so they resolve together with the including file's blocks.
func parseIncludes(pattern, baseDir string, seen map[string]bool, rootLine int) ([]*block, []string) {
	var blocks []*block
	var aliases []string
	for _, token := range strings.Fields(pattern) {
		expanded := expanduser(token)
		full := expanded
		if !filepath.IsAbs(expanded) {
			full = filepath.Join(baseDir, expanded)
		}
		matches, _ := filepath.Glob(full)
		sort.Strings(matches)
		for _, match := range matches {
			abs, err := filepath.Abs(match)
			if err != nil {
				continue
			}
			reached := abs
			if resolved, e := filepath.EvalSymlinks(abs); e == nil {
				abs = resolved
			}
			if seen[abs] || !isFile(abs) {
				continue
			}
			seen[abs] = true
			b, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			bs, as := collectBlocks(string(b), reached, baseDir, seen, false, rootLine)
			blocks = append(blocks, bs...)
			aliases = append(aliases, as...)
		}
	}
	return blocks, aliases
}

// resolution is how one parsed host maps onto the canonical layout.
type resolution struct {
	keyName   string
	adoptFrom string // source key to copy into profiles/<p>/ ("" if none)
	probe     string // existing key to fingerprint ("" if none)
}

// Importer onboards an ssh config.
type Importer struct {
	p               paths.Paths
	emitUseKeychain bool
	ks              *keystore.KeyStore
	// root is the folder being imported when it is not this machine's
	// ~/.ssh: an IdentityFile under ~/.ssh/ is looked for in it instead.
	root string
	// home is the ~/.ssh the import stands for, when p is a copy of it
	// (see InPlaceOf); empty, it is p's own.
	home string
	// standIn adopts a key as an empty file at its new path instead of a
	// copy of it, so a preview's copy never holds a private key.
	standIn bool
}

// InPlaceOf makes the import read as it would for real, a home p is a
// preview's copy of. The source is compared with real's ~/.ssh, not the
// copy's, so an IdentityFile resolves where it will on apply; and a key it
// would adopt is stood in for by an empty file, which says the key would be
// created without copying it.
func (im *Importer) InPlaceOf(real paths.Paths) *Importer {
	if real.SSHDir != "" && real.SSHDir != im.p.SSHDir {
		im.home, im.standIn = real.SSHDir, true
	}
	return im
}

// Source is what an import reads: the config files, in order, and the folder
// they belong to.
type Source struct {
	Root  string
	Files []string
}

// maxConfigDepth is how deep Locate looks for config files in a folder with
// no top-level config: profiles/<category>/<profile>/config is four levels.
const maxConfigDepth = 4

// Locate finds the ssh config to import at path:
//
//   - a file: that file;
//   - a folder with a config file: that file, whose Includes pull in the rest
//     (~/.ssh, or another machine's copy of it);
//   - a folder with ssh/config: that file, for a config home such as ~/.config;
//   - any other folder: every file named config, or ending .conf, up to four
//     levels down, in path order (a flat folder of *.conf files, a
//     config.d/, profiles/<name>/config), read as if one Included the next.
func Locate(path string) (Source, error) {
	path = expanduser(path)
	fi, err := os.Stat(path)
	if err != nil {
		return Source{}, &importError{"no ssh config to import at " + path}
	}
	if fi.Mode().IsRegular() {
		return Source{Root: filepath.Dir(path), Files: []string{path}}, nil
	}
	if !fi.IsDir() {
		return Source{}, &importError{path + " is neither a file nor a folder"}
	}
	for _, rel := range []string{"config", filepath.Join("ssh", "config")} {
		if c := filepath.Join(path, rel); isFile(c) {
			return Source{Root: filepath.Dir(c), Files: []string{c}}, nil
		}
	}
	var files []string
	base := strings.Count(filepath.Clean(path), string(filepath.Separator))
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(filepath.Clean(p), string(filepath.Separator))-base >= maxConfigDepth {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.Type().IsRegular() && (name == "config" || strings.HasSuffix(name, ".conf")) {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	if len(files) == 0 {
		return Source{}, &importError{"no ssh config to import under " + path +
			" (looked for config, ssh/config, and config or *.conf files up to " + strconv.Itoa(maxConfigDepth) + " levels down)"}
	}
	return Source{Root: path, Files: files}, nil
}

// parseSources parses every file of src as one config: each file's blocks
// in order, then every host resolved against all of them, as ssh resolves a
// config whose Includes pulled the rest in.
func parseSources(src Source) []*ParsedHost {
	hosts, _ := parseSourceBlocks(src)
	return hosts
}

// parseSourceBlocks is parseSources returning the blocks too, in the order
// ssh reads them, so import can tell which hand-written block defines what.
func parseSourceBlocks(src Source) ([]*ParsedHost, []*block) {
	seen := map[string]bool{}
	var blocks []*block
	var aliases []string
	for n, f := range src.Files {
		abs, err := filepath.Abs(f)
		if err != nil {
			continue
		}
		reached := abs
		if resolved, e := filepath.EvalSymlinks(abs); e == nil {
			abs = resolved
		}
		if seen[abs] {
			continue // already Included by a file read before it
		}
		seen[abs] = true
		text, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		bs, as := collectBlocks(string(text), reached, src.Root, seen, true, -1)
		if n == 0 {
			for _, b := range bs {
				b.first = true
			}
		}
		blocks = append(blocks, bs...)
		aliases = append(aliases, as...)
	}
	extras, folded := nameGroups(blocks)
	hosts := make([]*ParsedHost, 0, len(aliases))
	done := map[string]bool{}
	for _, alias := range aliases {
		if done[alias] || folded[alias] {
			continue
		}
		done[alias] = true
		h := resolveHost(alias, blocks)
		h.Aliases = extras[alias]
		hosts = append(hosts, h)
	}
	return hosts, blocks
}

// nameGroups reads the Host lines that give one host more than one name
// (Host web1 web1-cf): the first name is the host, and each other name that
// no block names first is one of its aliases rather than a host of its own.
// folded holds those other names.
func nameGroups(blocks []*block) (extras map[string][]string, folded map[string]bool) {
	extras, folded = map[string][]string{}, map[string]bool{}
	first := map[string]bool{}
	for _, b := range blocks {
		if lit, wild := literals(b.patterns); !b.all && len(wild) == 0 && len(lit) > 0 {
			first[lit[0]] = true
		}
	}
	for _, b := range blocks {
		lit, wild := literals(b.patterns)
		if b.all || len(wild) > 0 || len(lit) < 2 || b.include || folded[lit[0]] {
			continue
		}
		for _, n := range lit[1:] {
			if !first[n] && !folded[n] && n != lit[0] {
				folded[n] = true
				extras[lit[0]] = append(extras[lit[0]], n)
			}
		}
	}
	return extras, folded
}

// isBanner is a comment authama writes around what it renders - the
// managed header, a profile banner - which is no host's section.
func isBanner(line string) bool {
	t := strings.TrimSpace(strings.TrimLeft(line, "#"))
	return renderer.IsManagedHeaderLine(line) || renderer.IsManagedEndLine(line) || strings.HasPrefix(t, "---") ||
		strings.HasPrefix(t, "Managed by ") || strings.HasPrefix(t, "Category: ") || strings.HasPrefix(t, "Git (")
}

// New builds an importer.
func New(p paths.Paths, emitUseKeychain bool) *Importer {
	return &Importer{p: p, emitUseKeychain: emitUseKeychain, ks: keystore.New()}
}

// Run parses configPath into a fresh manifest + inventory (replacing any existing).
// With dryRun it parses and reports without writing or adopting keys.
//
// configPath may be a config file or any folder holding ssh config files; see
// Locate for how a folder is read.
func (im *Importer) Run(configPath string, dryRun bool) (ImportResult, error) {
	src, err := Locate(configPath)
	if err != nil {
		return ImportResult{}, err
	}
	for _, f := range src.Files {
		if _, err := os.ReadFile(f); err != nil {
			return ImportResult{}, &importError{"cannot read " + f + ": " + err.Error()}
		}
	}
	im.root = ""
	home := im.p.SSHDir
	if im.home != "" {
		home = im.home
	}
	if real, err := filepath.Abs(home); err == nil {
		if r, err := filepath.Abs(src.Root); err == nil && filepath.Clean(r) != filepath.Clean(real) && filepath.Base(r) != "ssh" {
			im.root = r
		}
	}
	parsed, blocks := parseSourceBlocks(src)
	// Parsing reads profiles off paths under the default root. A home whose
	// keys live under another (defaults.key_root) has its paths read again
	// under that root, and the new manifest keeps it.
	root := manifest.KeyRootAt(im.p.Manifest())
	if root != manifest.DefaultKeyRoot {
		for _, h := range parsed {
			if h.IdentityFile == "" {
				continue
			}
			if p, c := profileFromIdentityIn(h.IdentityFile, root); p != "imported" {
				h.Profile, h.Category = p, c
			}
		}
	}

	// A host whose hand-written block import cannot take out of the config
	// it read stays the user's, and is not imported: both would define it,
	// and ssh would read whichever comes first (review R-BUG-09). Leaving one
	// out can strand a block that names it beside another alias, so this
	// runs until nothing more is left out.
	plan := im.newAdoption(src, blocks)
	var b built
	for {
		if b, err = im.build(parsed, root, plan.excluded); err != nil {
			return ImportResult{}, err
		}
		if !plan.exclude(blocks, b, im) {
			break
		}
	}
	plan.place(blocks, b)
	if plan.insertAt >= 0 {
		carryGlobalOptions(b.m, blocks)
	}
	m := b.m

	inv := inventory.New()
	adopted := 0
	skipped := b.skipped
	for _, r := range b.resolved {
		ident := m.IdentityFile(r.h.Profile, r.res.keyName)
		if !dryRun && r.res.adoptFrom != "" {
			ok, why := im.adoptKey(m, r.res.adoptFrom, r.h.Profile, r.res.keyName)
			if ok {
				adopted++
			} else if why != "" {
				skipped = append(skipped, r.h.Alias+": key not adopted: "+why)
			}
		}
		if r.res.probe != "" {
			if fp := im.safeFingerprint(r.res.probe); fp != "" {
				inv.Record(fp, inventory.KeyRecord{
					Profile: r.h.Profile, Path: ident, Type: "ed25519", RotateAfterDays: 365,
				})
				inv.NoteAdded(fp, inventory.OriginImported, inventory.Stamp())
			}
		}
	}

	// Counted from what actually reached the manifest, not from what was parsed.
	// Duplicate Host blocks are dropped above (first wins) - appending to an ssh
	// config rather than editing it in place is normal, so a repeated alias is
	// normal - and counting parsed hosts reported an import larger than the one
	// written, with nothing in the summary to say which number was real.
	result := ImportResult{DryRun: dryRun, KeysFound: len(inv.Keys), Adopted: adopted, Profiles: map[string]int{}, Skipped: skipped,
		Sources: src.Files, Root: src.Root}
	for name, hosts := range b.byProfile {
		result.Profiles[name] = len(hosts)
	}
	result.Moved = plan.moved
	result.MovedFrom = plan.files()
	result.Kept = plan.keptLines()
	result.NotImported = notImported(src.Files)
	if !dryRun {
		if err := m.Save(im.p.Manifest()); err != nil {
			return ImportResult{}, err
		}
		if err := inv.Save(im.p.Inventory()); err != nil {
			return ImportResult{}, err
		}
		backup, err := plan.apply(im, m)
		if err != nil {
			return result, err
		}
		result.Backup = backup
	}
	return result, nil
}

// built is the manifest an import would write, and how it got there.
type built struct {
	m         *manifest.Manifest
	byProfile map[string][]manifest.Host
	resolved  []resolvedHost
	skipped   []string
	// unheld are hosts left out because their block holds what a manifest
	// host cannot (a ProxyCommand authama cannot quote), with the reason.
	unheld map[string]string
}

type resolvedHost struct {
	h   *ParsedHost
	res resolution
}

// build turns the parsed hosts, less those excluded, into a manifest. It
// refuses one Load would reject.
func (im *Importer) build(parsed []*ParsedHost, root string, excluded map[string]string) (built, error) {
	m := manifest.Starter(im.emitUseKeychain) // defaults with the right global options
	if root != manifest.DefaultKeyRoot {
		m.Defaults.KeyRoot = root
	}
	out := built{m: m, byProfile: map[string][]manifest.Host{}, unheld: map[string]string{}}
	order := []string{}
	category := map[string]string{} // profile -> the first category its keys were found under
	seen := map[string]bool{}
	for _, h := range parsed {
		// Keyed on the alias alone, not profile+alias: a repeated alias whose
		// blocks name keys in two profiles is still one ssh host (the first
		// block wins), and the manifest rejects an alias declared twice in any
		// profiles. The parser already yields each alias once; this keeps the
		// manifest valid even if that ever changes.
		if seen[h.Alias] {
			continue
		}
		seen[h.Alias] = true
		if _, gone := excluded[h.Alias]; gone {
			continue
		}
		res := im.resolveKey(h)
		kn := res.keyName
		hostName := h.Hostname
		if hostName == "" {
			hostName = h.Alias
		}
		user := h.User
		if user == "" {
			if user = os.Getenv("USER"); user == "" {
				user = "git"
			}
		}
		host := manifest.Host{
			Alias: h.Alias, Hostname: hostName, User: user, Port: h.Port,
			Provider: inferProvider(hostName), KeyName: &kn,
			RawOptions: manifest.NewOrderedOptions(pairs(h.Extra)),
			Aliases:    h.Aliases, Section: h.Section, Comment: h.Comment,
		}
		if h.ProxyCommand != "" {
			proxy, err := manifest.ProxyFromCommand(h.ProxyCommand)
			if err != nil {
				why := "its ProxyCommand cannot be held: " + err.Error()
				out.skipped = append(out.skipped, h.Alias+": "+why)
				out.unheld[h.Alias] = why
				continue
			}
			host.Proxy = proxy
		}
		// One host the manifest would refuse is left out and reported; the
		// rest of the config is still imported.
		if err := manifest.ValidateHost(h.Profile, host); err != nil {
			out.skipped = append(out.skipped, h.Alias+": "+err.Error())
			continue
		}
		out.resolved = append(out.resolved, resolvedHost{h, res})
		if _, ok := out.byProfile[h.Profile]; !ok {
			order = append(order, h.Profile)
		}
		if category[h.Profile] == "" {
			category[h.Profile] = h.Category
		}
		out.byProfile[h.Profile] = append(out.byProfile[h.Profile], host)
	}
	for _, name := range order {
		m.SetProfile(name, manifest.Profile{Category: category[name], KeyScope: "per_service", Hosts: out.byProfile[name]})
	}
	// Save does not validate, and every later command Loads (which does). An
	// import that wrote a manifest Load then rejects would report success and
	// leave authama unusable, so refuse it here - before any key is adopted, and
	// in a dry run too, which should predict the real run.
	if err := m.Validate(); err != nil {
		return built{}, &importError{"import would produce an invalid manifest: " + err.Error()}
	}
	return out, nil
}

func (im *Importer) resolveKey(h *ParsedHost) resolution {
	if h.IdentityFile == "" {
		kn, _ := key.DeriveKeyName(h.Profile, h.Alias, "ed25519")
		return resolution{keyName: kn}
	}
	real := im.identityPath(h.IdentityFile)
	pub := real + ".pub"
	probe := ""
	switch {
	case isFile(pub):
		probe = pub
	case exists(real):
		probe = real
	}
	if underProfile(real, h.Profile, manifest.KeyRootAt(im.p.Manifest())) {
		return resolution{keyName: filepath.Base(real), probe: probe}
	}
	kn, _ := key.DeriveKeyName(h.Profile, h.Alias, "ed25519")
	adopt := ""
	if exists(real) {
		adopt = real
	}
	return resolution{keyName: kn, adoptFrom: adopt, probe: probe}
}

// identityPath is where an IdentityFile's key is on this machine. Importing
// another machine's ~/.ssh, a path under ~/.ssh/ names a file in that copy,
// which is looked for there first.
func (im *Importer) identityPath(identity string) string {
	if im.root != "" {
		norm := strings.ReplaceAll(identity, `\`, "/")
		if rest, ok := strings.CutPrefix(norm, "~/.ssh/"); ok {
			if c := filepath.Join(im.root, filepath.FromSlash(rest)); exists(c) {
				return c
			}
		}
	}
	return expanduser(identity)
}

func underProfile(real, profile, root string) bool {
	got, _ := profileFromIdentityIn(real, root)
	return got == profile && strings.Contains(filepath.ToSlash(real), "/"+root+"/")
}

// adoptKey copies an existing keypair into the profile's directory
// (profiles/[<category>/]<profile>/<keyName>). Non-destructive: skips if the
// destination already exists. When it refuses for a reason the user should
// hear, why says it.
func (im *Importer) adoptKey(m *manifest.Manifest, srcPriv, profile, keyName string) (ok bool, why string) {
	dstPriv := m.KeyPath(im.p.SSHDir, manifest.KeyRef{Profile: profile, KeyName: keyName})
	// occupied, not exists: a dangling symlink at the destination reads as
	// absent through Stat, and the copy would then follow it out of the tree.
	if occupied(dstPriv) || !exists(srcPriv) {
		return false, ""
	}
	// MkdirAll and the chmod below follow a link anywhere on the way, so a
	// profile directory that is a symlink had its target made 0700, wherever
	// it pointed (review A-SEC-15). The key itself is written O_EXCL, never
	// through a link; its directory is held to the same rule.
	if link := linkUnder(im.p.SSHDir, filepath.Dir(dstPriv)); link != "" {
		return false, link + " is a symbolic link; authama will not write a key through it"
	}
	if err := os.MkdirAll(filepath.Dir(dstPriv), perms.DirMode); err != nil {
		return false, ""
	}
	// A link swapped in between the check and here is caught again before the chmod.
	if link := linkUnder(im.p.SSHDir, filepath.Dir(dstPriv)); link != "" {
		return false, link + " is a symbolic link; authama will not write a key through it"
	}
	_ = perms.SetPerms(filepath.Dir(dstPriv), perms.DirMode)
	if im.standIn {
		if writeNew(dstPriv, nil) != nil {
			return false, ""
		}
	} else if !copyFile(srcPriv, dstPriv) {
		return false, ""
	}
	_ = perms.SetPerms(dstPriv, perms.PrivateKeyMode)
	// The public half replaces whatever is beside the new private key, a stale
	// .pub included, by rename: a rename replaces a symlink there rather than
	// writing through it.
	if b, err := os.ReadFile(srcPriv + ".pub"); err == nil {
		if fs.WriteTextAtomic(dstPriv+".pub", string(b), perms.PublicKeyMode) == nil {
			_ = perms.SetPerms(dstPriv+".pub", perms.PublicKeyMode)
		}
	}
	return true, ""
}

// linkUnder returns the first directory from base (exclusive) down to dir
// (inclusive) that is a symbolic link, or "" when none is. base itself may be
// a link - a ~/.ssh kept in a dotfiles checkout is ordinary - but nothing
// authama creates under it may be. A component that does not exist yet is
// not a link.
func linkUnder(base, dir string) string {
	rel, err := filepath.Rel(base, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	cur := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err != nil {
			return ""
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return cur
		}
	}
	return ""
}

func (im *Importer) safeFingerprint(probe string) string {
	fp, err := im.ks.Fingerprint(probe)
	if err != nil {
		return ""
	}
	return fp
}

// --- small helpers ---------------------------------------------------------

type importError struct{ msg string }

func (e *importError) Error() string { return e.msg }

func pairs(extra []kv) [][2]string {
	out := make([][2]string, len(extra))
	for i, e := range extra {
		// The parser lower-cases keywords; the manifest keeps OpenSSH's spelling.
		out[i] = [2]string{manifest.CanonicalOption(e.Key), e.Val}
	}
	return out
}

// expanduser resolves a leading ~ against the user's home directory.
//
// Both separators are accepted. An ssh config written on Windows can say
// `~\.ssh\id_ed25519`, and honouring only "~/" left that as a literal path
// beginning with a tilde - which then failed to glob, so the key was silently
// skipped and the host imported without its identity.
func expanduser(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, filepath.FromSlash(strings.ReplaceAll(p[2:], `\`, "/")))
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// occupied reports whether anything is at path, a symlink included, even one
// whose target is missing.
func occupied(path string) bool { _, err := os.Lstat(path); return err == nil }

// copyFile copies src to a new file at dst, never through anything already
// there: see writeNew.
func copyFile(src, dst string) bool {
	b, err := os.ReadFile(src)
	if err != nil {
		return false
	}
	return writeNew(dst, b) == nil
}

// writeNew creates dst, owner-only, and fails if anything is there: O_EXCL
// refuses an existing file and does not follow a symlink, dangling or not.
func writeNew(dst string, b []byte) error {
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(dst)
		return err
	}
	return f.Close()
}

// IdentityFiles is every IdentityFile the ssh config at configPath names, in
// any block (pattern blocks included) and in any file it Includes, with ~
// expanded. Another package's check that a key "is used by no Host block"
// reads this, so it sees the hand-written blocks authama does not manage.
func IdentityFiles(configPath string) []string {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	blocks, _ := collectBlocks(string(b), configPath, filepath.Dir(configPath), map[string]bool{}, true, -1)
	var out []string
	for _, bl := range blocks {
		for _, o := range bl.opts {
			if strings.EqualFold(o.Key, "IdentityFile") {
				if arg := firstArg(o.Val); arg != "" {
					out = append(out, expanduser(arg))
				}
			}
		}
	}
	return out
}

// notImported lists, as written, the Match blocks and the Host blocks whose
// every pattern is a wildcard in files: what an import cannot turn into a
// manifest host, and so leaves in place.
func notImported(files []string) []string {
	var out []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, raw := range strings.Split(string(b), "\n") {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			keyword, value := splitKV(line)
			switch keyword {
			case "match":
				out = append(out, "Match "+value)
			case "host":
				wild := true
				for _, p := range argvSplit(value) {
					if !strings.ContainsAny(p, "*?!") {
						wild = false
					}
				}
				if wild {
					out = append(out, "Host "+value)
				}
			}
		}
	}
	return out
}
