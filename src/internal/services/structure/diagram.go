package structure

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// DiagramLine is one line of the ~/.ssh diagram: the tree drawing and the
// name, and what it is.
type DiagramLine struct {
	Text, Comment string
}

// FolderDiagram is ~/.ssh as a text tree, as it is on disk, each line
// labelled from the manifest, and the counts under it.
type FolderDiagram struct {
	Lines []DiagramLine
	// Groups are the categories under the key root (personal, orgs, …),
	// Owners the profile folders in them, Pairs the private keys with their
	// .pub, KeyFiles every key file (both halves, and a lone .pub), and
	// Configs the config, known_hosts and authorized_keys files.
	Groups, Owners, Pairs, KeyFiles, Configs int
}

// String is the diagram as text: each comment aligned in one column, then
// the counts.
func (d FolderDiagram) String() string {
	width := 0
	for _, l := range d.Lines {
		if n := len([]rune(l.Text)); l.Comment != "" && n > width {
			width = n
		}
	}
	var b strings.Builder
	for _, l := range d.Lines {
		b.WriteString(l.Text)
		if l.Comment != "" {
			b.WriteString(strings.Repeat(" ", width-len([]rune(l.Text))+4))
			b.WriteString("# " + l.Comment)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + d.Summary() + "\n")
	return b.String()
}

// Summary is the counts line under the diagram.
func (d FolderDiagram) Summary() string {
	return fmt.Sprintf("%s · %s · %s (%s) · %s",
		plural(d.Groups, "profile group", "profile groups"), plural(d.Owners, "owner", "owners"),
		plural(d.Pairs, "key pair", "key pairs"), plural(d.KeyFiles, "key file", "key files"),
		plural(d.Configs, "config/host file", "config/host files"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// sshFileNotes say what the files OpenSSH itself reads are.
var sshFileNotes = map[string]string{
	"config":           "root config, includes the profile configs",
	"known_hosts":      "trusted host fingerprints",
	"known_hosts.old":  "the previous known_hosts",
	"authorized_keys":  "keys allowed to log in here",
	"authorized_keys2": "keys allowed to log in here (legacy name)",
	"authama-cm":       "shared ssh connections (ControlMaster sockets)",
	"agent":            "ssh-agent socket",
}

// Diagram reads ~/.ssh under p as it is: every folder and file but dot-files,
// sockets and rotation archives (old/), a key's two halves on one line as
// name{,.pub}. A key line says which hosts use it, or which profile owns it
// when it is a tool key; nothing on disk is changed.
func Diagram(p paths.Paths) (FolderDiagram, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil && !os.IsNotExist(err) {
		return FolderDiagram{}, err
	}
	if m == nil {
		m = &manifest.Manifest{}
	}
	d := &diagram{p: p, m: m, root: filepath.Join(p.SSHDir, filepath.FromSlash(manifest.KeyRoot(m)))}
	d.out.Lines = append(d.out.Lines, DiagramLine{Text: "~/.ssh/"})
	if err := d.walk(p.SSHDir, "", 0); err != nil {
		return FolderDiagram{}, err
	}
	return d.out, nil
}

type diagram struct {
	p    paths.Paths
	m    *manifest.Manifest
	root string // the key root, ~/.ssh/profiles
	out  FolderDiagram
}

// entry is one line to draw in a folder: a file, a key pair, or a folder.
type entry struct {
	name string // as drawn: "x{,.pub}" for a pair
	path string // the private key for a pair, else the file or folder
	dir  bool
	pair bool
	key  bool // a key file (either half)
}

func (d *diagram) walk(dir, prefix string, depth int) error {
	if depth > 6 {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := map[string]bool{}
	for _, e := range ents {
		names[e.Name()] = true
	}
	var files, dirs []entry
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "old" {
			continue
		}
		full := filepath.Join(dir, name)
		fi, err := os.Lstat(full)
		if err != nil {
			continue
		}
		switch {
		case fi.IsDir():
			dirs = append(dirs, entry{name: name + "/", path: full, dir: true})
		case !fi.Mode().IsRegular():
			continue // a socket (ControlMaster, an agent) or a symlink
		case strings.HasSuffix(name, ".pub"):
			if names[strings.TrimSuffix(name, ".pub")] {
				continue // drawn with its private half
			}
			files = append(files, entry{name: name, path: full, key: true})
		case names[name+".pub"]:
			files = append(files, entry{name: name + "{,.pub}", path: full, pair: true, key: true})
		default:
			files = append(files, entry{name: name, path: full, key: looksLikeKey(full)})
		}
	}
	sort.Slice(files, func(i, j int) bool { return lessFile(files[i].name, files[j].name) })
	// The shared tools folder comes last under the key root, after the
	// categories, as the owner draws it.
	sort.Slice(dirs, func(i, j int) bool {
		ti, tj := d.isToolsRoot(dirs[i].path), d.isToolsRoot(dirs[j].path)
		if ti != tj {
			return tj
		}
		return dirs[i].name < dirs[j].name
	})
	all := append(files, dirs...)
	for i, e := range all {
		last := i == len(all)-1
		branch, next := "├── ", "│   "
		if last {
			branch, next = "└── ", "    "
		}
		d.out.Lines = append(d.out.Lines, DiagramLine{Text: prefix + branch + e.name, Comment: d.comment(e)})
		d.count(e)
		if e.dir {
			if err := d.walk(e.path, prefix+next, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

// lessFile orders a folder's files as OpenSSH's own come first: config,
// known_hosts, authorized_keys, then the rest by name.
func lessFile(a, b string) bool {
	rank := func(n string) int {
		switch {
		case n == "config":
			return 0
		case strings.HasPrefix(n, "known_hosts"):
			return 1
		case strings.HasPrefix(n, "authorized_keys"):
			return 2
		}
		return 3
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra < rb
	}
	return a < b
}

func (d *diagram) isToolsRoot(path string) bool {
	return filepath.Clean(path) == filepath.Join(d.root, manifest.ToolsDir)
}

func (d *diagram) count(e entry) {
	parent := filepath.Dir(e.path)
	switch {
	case e.dir && parent == d.root && !d.isToolsRoot(e.path):
		if d.isCategory(e.path) {
			d.out.Groups++
		} else {
			d.out.Owners++ // an uncategorised profile's folder
		}
	case e.dir && filepath.Dir(parent) == d.root && !d.isToolsRoot(parent) && d.isCategory(parent):
		d.out.Owners++
	case e.pair:
		d.out.Pairs++
		d.out.KeyFiles += 2
	case e.key:
		d.out.KeyFiles++
	case isConfigFile(filepath.Base(e.path)):
		d.out.Configs++
	}
}

func isConfigFile(name string) bool {
	return name == "config" || strings.HasPrefix(name, "known_hosts") || strings.HasPrefix(name, "authorized_keys")
}

// isCategory reports a folder under the key root that the manifest uses as a
// category (personal, orgs, …), as opposed to an uncategorised profile.
func (d *diagram) isCategory(path string) bool {
	name := filepath.Base(path)
	for _, c := range d.m.Categories() {
		if c == name {
			return true
		}
	}
	_, isProfile := d.m.Profiles[name]
	return !isProfile
}

func (d *diagram) comment(e entry) string {
	parent := filepath.Dir(e.path)
	base := filepath.Base(e.path)
	switch {
	case parent == d.p.SSHDir:
		if note, ok := sshFileNotes[base]; ok {
			return note
		}
	case e.dir && d.isToolsRoot(e.path):
		return "no config, used directly by tools"
	case !e.dir && base == "config" && filepath.Dir(parent) == d.root:
		return "host entries for " + filepath.Base(parent) + " profiles"
	}
	if !e.key {
		return ""
	}
	priv := strings.TrimSuffix(e.path, ".pub")
	loc, ok := manifest.LocateKeyPath(d.m, d.p.SSHDir, priv)
	if !ok {
		if parent == d.p.SSHDir {
			return "not in the manifest"
		}
		return ""
	}
	ref := manifest.KeyRef{Profile: loc.Profile, KeyName: loc.Name}
	if spec, ok := d.m.KeySpecFor(ref); ok && spec.EffectiveRole() == manifest.RoleTool {
		owner := loc.Profile
		if c := d.m.Profiles[loc.Profile].Category; c != "" {
			owner = c + "/" + loc.Profile
		}
		what := spec.ToolKind
		if what == "" {
			what = spec.Tool
		}
		return what + ", owned by " + owner
	}
	hosts, err := d.m.HostsForKey(ref)
	if err != nil || len(hosts) == 0 {
		return ""
	}
	var aliases []string
	for _, h := range hosts {
		aliases = append(aliases, h.Alias)
	}
	return "used by " + strings.Join(aliases, ", ")
}

// looksLikeKey reports a file that holds a private key, by its first line.
func looksLikeKey(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	return strings.Contains(string(buf[:n]), "PRIVATE KEY")
}
