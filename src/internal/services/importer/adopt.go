package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/homeperms"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// adoption is what an import does to the hand-written config it read.
//
// Importing a host used to copy it into the manifest and leave its block in
// ~/.ssh/config, so after the next render ssh found two definitions of it and
// doctor failed on the documented onboarding (review R-BUG-09). Import now
// adopts: a block whose every line the rendered host reproduces is taken out of
// the file in the same run that writes the manifest, and the managed block is
// rendered where it was. A block it cannot take out leaves its host the
// user's, not imported, so there is never a second definition to report.
type adoption struct {
	// live is set when the source is the ssh config this machine reads (or a
	// file under its ~/.ssh): only then is anything edited or left out. An
	// import of another machine's copy edits nothing, as before.
	live    bool
	liveDir string // that ~/.ssh
	editDir string // where edits land: liveDir, or a preview's copy of it
	// rootFile is the live ~/.ssh/config when the source starts there, as
	// reached; "" otherwise. Only it gets the managed block.
	rootFile string
	excluded map[string]string // alias -> why it stays the user's
	order    []string          // excluded aliases, in the order found
	remove   map[string][]renderer.LineRange
	moved    []string
	insertAt int // line of rootFile the managed block takes; -1 to leave it be
}

func (im *Importer) newAdoption(src Source, blocks []*block) *adoption {
	a := &adoption{excluded: map[string]string{}, remove: map[string][]renderer.LineRange{}, insertAt: -1}
	a.liveDir, a.editDir = im.p.SSHDir, im.p.SSHDir
	if im.home != "" {
		a.liveDir = im.home
	}
	if im.root != "" || a.liveDir == "" {
		return a
	}
	for _, f := range src.Files {
		if abs, err := filepath.Abs(f); err == nil && a.under(abs) {
			a.live = true
		}
	}
	if len(src.Files) > 0 {
		if abs, err := filepath.Abs(src.Files[0]); err == nil {
			if rel, ok := a.rel(abs); ok && rel == renderer.RootConfig {
				a.rootFile = abs
			}
		}
	}
	return a
}

// rel is file's path relative to the live ~/.ssh, when it is inside it. Both
// spellings are tried, so a temp dir under /var that is /private/var matches.
func (a *adoption) rel(file string) (string, bool) {
	try := func(dir, f string) (string, bool) {
		r, err := filepath.Rel(dir, f)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
			return "", false
		}
		return r, true
	}
	if r, ok := try(a.liveDir, file); ok {
		return r, true
	}
	dir, err1 := filepath.EvalSymlinks(a.liveDir)
	parent, err2 := filepath.EvalSymlinks(filepath.Dir(file))
	if err1 != nil || err2 != nil {
		return "", false
	}
	return try(dir, filepath.Join(parent, filepath.Base(file)))
}

func (a *adoption) under(file string) bool { _, ok := a.rel(file); return ok }

// editPath is where an edit of file lands: file itself, or its counterpart in
// a preview's copy of ~/.ssh.
func (a *adoption) editPath(file string) string {
	r, _ := a.rel(file)
	return filepath.Join(a.editDir, r)
}

func (a *adoption) leave(alias, why string) bool {
	if _, done := a.excluded[alias]; done {
		return false
	}
	a.excluded[alias] = why
	a.order = append(a.order, alias)
	return true
}

func literals(patterns []string) (lit, wild []string) {
	for _, p := range patterns {
		if strings.ContainsAny(p, "*?!") {
			wild = append(wild, p)
		} else {
			lit = append(lit, p)
		}
	}
	return lit, wild
}

// exclude leaves out each host whose hand-written block cannot be moved, and
// reports whether it left out any it had not before. b is the manifest built
// without the ones already left out.
func (a *adoption) exclude(blocks []*block, b built, im *Importer) bool {
	if !a.live {
		return false
	}
	hosts := map[string]*resolvedHost{}
	for i := range b.resolved {
		hosts[b.resolved[i].h.Alias] = &b.resolved[i]
	}
	changed := false
	leaveAll := func(lit []string, why string) {
		for _, l := range lit {
			if a.leave(l, why) {
				changed = true
			}
		}
	}
	for i, bl := range blocks {
		if bl.all || bl.managed {
			continue
		}
		lit, wild := literals(bl.patterns)
		if len(lit) == 0 {
			continue
		}
		switch {
		case !a.under(bl.file):
			leaveAll(lit, fmt.Sprintf("defined in %s, which authama does not edit", bl.file))
			continue
		case len(wild) > 0:
			leaveAll(lit, fmt.Sprintf("its Host line also has the pattern %s, which a managed host cannot hold", strings.Join(wild, " ")))
			continue
		case bl.include:
			leaveAll(lit, "its Host block holds an Include, which a managed host cannot hold")
			continue
		}
		for _, l := range lit {
			if hosts[l] == nil {
				// Skipped, or already left out: the block stays, so every
				// alias it names stays with it.
				var others []string
				for _, o := range lit {
					if o != l {
						others = append(others, o)
					}
				}
				leaveAll(others, "it shares its Host line with "+l+", which is not imported")
				break
			}
		}
		for _, l := range lit {
			rh := hosts[l]
			if rh == nil {
				continue
			}
			host, profile := hostIn(b.m, l)
			ident := im.identityPath(rh.h.IdentityFile)
			sameKey := func(p string) bool { return rh.h.IdentityFile != "" && filepath.Clean(p) == filepath.Clean(ident) }
			for _, o := range bl.opts {
				if !renderer.MultiValued(o.Key) && setEarlier(blocks[:i], l, o.Key) {
					continue // ssh already has a value for it: this line is never read
				}
				if ok, why := renderer.Represents(b.m, profile, host, renderer.OptionLine{Key: o.Key, Value: o.Val}, sameKey); !ok {
					leaveAll(lit, why)
					break
				}
			}
		}
	}
	return changed
}

// setEarlier reports whether a block before this one that applies to alias
// sets key, which makes ssh ignore a later single value for it.
func setEarlier(before []*block, alias, key string) bool {
	for _, bl := range before {
		if !bl.matches(alias) {
			continue
		}
		for _, o := range bl.opts {
			if strings.EqualFold(o.Key, key) {
				return true
			}
		}
	}
	return false
}

func hostIn(m *manifest.Manifest, alias string) (manifest.Host, string) {
	for _, pname := range m.ProfileNames() {
		for _, h := range m.Profiles[pname].Hosts {
			if h.Alias == alias {
				return h, pname
			}
		}
	}
	return manifest.Host{}, ""
}

// place picks the blocks to take out, now that the excluded set is final, and
// where in ~/.ssh/config the managed block goes.
func (a *adoption) place(blocks []*block, b built) {
	if !a.live {
		return
	}
	in := map[string]bool{}
	for _, r := range b.resolved {
		in[r.h.Alias] = true
	}
	seen := map[string]bool{}
	for _, bl := range blocks {
		if bl.all || bl.managed || bl.include || !a.under(bl.file) {
			continue
		}
		lit, wild := literals(bl.patterns)
		if len(lit) == 0 || len(wild) > 0 {
			continue
		}
		ok := true
		for _, l := range lit {
			if _, out := a.excluded[l]; out || !in[l] {
				ok = false
			}
		}
		if !ok {
			continue
		}
		a.remove[bl.file] = append(a.remove[bl.file], renderer.LineRange{Start: bl.start, End: bl.end})
		for _, l := range lit {
			if !seen[l] {
				seen[l] = true
				a.moved = append(a.moved, l)
			}
		}
		if a.rootFile != "" && bl.first {
			at := bl.rootLine
			if a.insertAt < 0 || at < a.insertAt {
				a.insertAt = at
			}
		}
	}
	sort.Strings(a.moved)
}

func (a *adoption) files() []string {
	out := make([]string, 0, len(a.remove))
	for f := range a.remove {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

func (a *adoption) keptLines() []string {
	out := make([]string, 0, len(a.order))
	for _, alias := range a.order {
		out = append(out, alias+" - "+a.excluded[alias])
	}
	return out
}

// apply takes the moved blocks out of their files and renders the managed
// block in the same pass, so ssh is never left without the hosts it moved.
// Each file is copied to a backup first and replaced atomically. It returns
// the backup folder, "" when nothing was edited.
func (a *adoption) apply(im *Importer, m *manifest.Manifest) (string, error) {
	if !a.live || len(a.remove) == 0 {
		return "", nil
	}
	backup := filepath.Join(im.p.StateDir(), "import-backup-"+time.Now().Format("20060102-150405"), "ssh")
	edits := map[string]string{}
	for file, ranges := range a.remove {
		dst := a.editPath(file)
		b, err := os.ReadFile(dst)
		if err != nil {
			return "", fmt.Errorf("could not read %s to move the imported hosts out of it: %w", dst, err)
		}
		rel, _ := a.rel(file)
		saved := filepath.Join(backup, rel)
		if err := os.MkdirAll(filepath.Dir(saved), homeperms.DirMode); err != nil {
			return "", err
		}
		if err := fs.WriteTextAtomic(saved, string(b), perms.ConfigMode); err != nil {
			return "", fmt.Errorf("could not back %s up, so it was not edited: %w", dst, err)
		}
		at := -1
		if file == a.rootFile {
			at = a.insertAt
		}
		edits[dst] = renderer.StripLines(string(b), ranges, at)
	}
	root := filepath.Join(a.editDir, renderer.RootConfig)
	if _, ok := edits[root]; !ok && a.rootFile != "" && a.insertAt >= 0 {
		// Only included files lost blocks: the managed block still goes where
		// the Include that read the first of them is.
		if b, err := os.ReadFile(root); err == nil {
			edits[root] = renderer.StripLines(string(b), nil, a.insertAt)
		}
	}
	svc := configsvc.New(a.editDir, m, im.emitUseKeychain)
	if a.editDir == im.p.SSHDir {
		svc = svc.WithState(im.p.StateDir()).Force(filepath.Join(backup, "hand-edits"))
	}
	rendered, err := svc.Rendered()
	if err != nil {
		return backup, err
	}
	// ~/.ssh/config gets its managed block in the same write that takes the
	// hand-written blocks out.
	if stripped, ok := edits[root]; ok {
		edits[root] = renderer.ComposeRootConfig(stripped, rendered[renderer.RootConfig])
	}
	for _, dst := range sortedPaths(edits) {
		if err := configsvc.WriteConfigFile(dst, edits[dst]); err != nil {
			return backup, fmt.Errorf("could not write %s (the original is in %s): %w", dst, backup, err)
		}
	}
	// The category files and anything else the render owns.
	if _, err := svc.Write(false); err != nil {
		return backup, err
	}
	log.Audit(im.p.AuditLog(), "import.adopt",
		log.Field{Key: "hosts", Value: strings.Join(a.moved, ",")},
		log.Field{Key: "backup", Value: backup})
	return backup, nil
}

func sortedPaths(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// carriedGlobals are the Host * options authama renders itself. Once the
// managed block takes the place of the first block import moved, a Host * the
// user wrote further down is read after authama's, which would win for these;
// the user's values are carried into defaults.global_options so ssh keeps
// using them.
var carriedGlobals = []string{"AddKeysToAgent", "IgnoreUnknown", "UseKeychain", "IdentitiesOnly",
	"ServerAliveInterval", "HashKnownHosts"}

func carryGlobalOptions(m *manifest.Manifest, blocks []*block) {
	done := map[string]bool{}
	for _, bl := range blocks {
		star := len(bl.patterns) == 1 && bl.patterns[0] == "*"
		if bl.managed || (!bl.all && !star) {
			continue
		}
		for _, o := range bl.opts {
			for _, k := range carriedGlobals {
				if !strings.EqualFold(o.Key, k) || done[k] {
					continue
				}
				done[k] = true
				if k == "UseKeychain" && !hasKey(m.Defaults.GlobalOptions, k) {
					continue // not rendered on this platform
				}
				m.Defaults.GlobalOptions.Set(k, o.Val)
			}
		}
	}
}

func hasKey(o manifest.OrderedOptions, k string) bool {
	for _, x := range o.Keys() {
		if strings.EqualFold(x, k) {
			return true
		}
	}
	return false
}
