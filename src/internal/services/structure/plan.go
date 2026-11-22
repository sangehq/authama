package structure

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/services/reconciler"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// Move kinds, as plan lines spell them.
const (
	MoveKey       = "key"       // key <src> profile <p> [name <n>]
	MoveMisplaced = "misplaced" // misplaced <profile>/<key>
	MoveCategory  = "profile"   // profile <name> category <cat|->
)

// Move is one approved line of a plan.
type Move struct {
	Kind     string
	Src      string // MoveKey: "~/.ssh/<rel>" or "config/<name>"
	Profile  string
	Name     string // MoveKey: the key name (defaults to the file's); MoveMisplaced: the key
	Category string // MoveCategory: "" for none
}

// Line is the move as a plan line.
func (mv Move) Line() string {
	switch mv.Kind {
	case MoveMisplaced:
		return "misplaced " + mv.Profile + "/" + mv.Name
	case MoveCategory:
		c := mv.Category
		if c == "" {
			c = "-"
		}
		return "profile " + mv.Profile + " category " + c
	}
	return KeyLine(mv.Src, mv.Profile, mv.Name)
}

// FromConfigHome reports whether the move takes a key out of the config home.
func (mv Move) FromConfigHome() bool {
	return mv.Kind == MoveKey && strings.HasPrefix(mv.Src, "config/")
}

// ParsePlan reads plan lines, one move each; blank lines and # comments are
// skipped. It checks the shape of each line, not the files.
func ParsePlan(text string) ([]Move, error) {
	var out []Move
	seen := map[string]bool{}
	for n, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		mv, err := parseLine(strings.Fields(ln))
		if err != nil {
			return nil, fmt.Errorf("plan line %d (%q): %w", n+1, ln, err)
		}
		if seen[mv.Line()] {
			continue
		}
		seen[mv.Line()] = true
		out = append(out, mv)
	}
	if len(out) == 0 {
		return nil, errors.New("the plan is empty: nothing to move")
	}
	return out, nil
}

func parseLine(f []string) (Move, error) {
	switch {
	case len(f) >= 4 && f[0] == MoveKey && f[2] == "profile":
		mv := Move{Kind: MoveKey, Src: f[1], Profile: f[3]}
		switch {
		case len(f) == 6 && f[4] == "name":
			mv.Name = f[5]
		case len(f) != 4:
			return Move{}, errors.New("want: key <path> profile <profile> [name <key>]")
		}
		if err := checkSrc(mv.Src); err != nil {
			return Move{}, err
		}
		if mv.Name == "" {
			mv.Name = path.Base(strings.TrimPrefix(mv.Src, "config/"))
		}
		return mv, nil
	case len(f) == 2 && f[0] == MoveMisplaced:
		prof, key, ok := strings.Cut(f[1], "/")
		if !ok || prof == "" || key == "" {
			return Move{}, errors.New("want: misplaced <profile>/<key>")
		}
		return Move{Kind: MoveMisplaced, Profile: prof, Name: key}, nil
	case len(f) == 4 && f[0] == MoveCategory && f[2] == "category":
		c := f[3]
		if c == "-" {
			c = ""
		}
		return Move{Kind: MoveCategory, Profile: f[1], Category: c}, nil
	}
	return Move{}, errors.New("not a move: want `key …`, `misplaced …` or `profile … category …`")
}

// checkSrc refuses a source that is not plainly a file in ~/.ssh or directly
// in the config home.
func checkSrc(src string) error {
	if rest, ok := strings.CutPrefix(src, "config/"); ok {
		if rest == "" || strings.ContainsAny(rest, `/\`) || strings.HasPrefix(rest, ".") {
			return fmt.Errorf("%s: name a file directly in the config home", src)
		}
		return nil
	}
	rel, ok := strings.CutPrefix(src, "~/.ssh/")
	if !ok {
		return fmt.Errorf("%s: a source is ~/.ssh/<path> or config/<name>", src)
	}
	if err := manifest.CheckAdoptedPath(rel); err != nil {
		return err
	}
	return nil
}

// srcAbs is where a move's source is under p.
func srcAbs(p paths.Paths, src string) string {
	if rest, ok := strings.CutPrefix(src, "config/"); ok {
		return filepath.Join(p.ConfigDir, rest)
	}
	return filepath.Join(p.SSHDir, filepath.FromSlash(strings.TrimPrefix(src, "~/.ssh/")))
}

// Dest is where a key move puts the private key under p, per the manifest
// as it stands (a category move earlier in the same plan is not reflected).
func Dest(p paths.Paths, m *manifest.Manifest, mv Move) string {
	return filepath.Join(m.ProfileDir(p.SSHDir, mv.Profile), mv.Name)
}

// Result is what Apply did.
type Result struct {
	Moved    []string // "src -> dest" lines
	Declared []string // profile/key
	Rewrote  []string // hand-written config files
	Backup   string   // where the moved files were copied first ("" when none moved)
}

// Apply carries out an approved plan under p: category moves first (through
// the editor, which moves the profile's directory and its records), then each
// key move - backed up to the state home's moved/<time>/, renamed into place,
// declared in the manifest, recorded, and every hand-written IdentityFile
// naming it repointed. Config-home sources are skipped here: MoveConfigHome
// lays them down first, on apply only (a preview's copy does not hold them),
// so here they are declared where they will be. Each move is audited as
// structure.move. Everything is checked before anything is touched.
func Apply(p paths.Paths, moves []Move, out io.Writer) (Result, error) {
	var res Result
	for _, mv := range moves {
		if mv.Kind != MoveCategory {
			continue
		}
		cr, err := editor.New(p).SetCategory(mv.Profile, mv.Category)
		if err != nil {
			return res, fmt.Errorf("%s: %w", mv.Line(), err)
		}
		_, _ = fmt.Fprintf(out, "profile %s: %s -> %s\n", mv.Profile, cr.From, cr.To)
		if cr.Moved {
			changed, err := rewriteDirRefs(p, filepath.Join(p.SSHDir, filepath.FromSlash(cr.From)),
				filepath.Join(p.SSHDir, filepath.FromSlash(cr.To)))
			if err != nil {
				return res, err
			}
			res.Rewrote = append(res.Rewrote, changed...)
		}
		audit(p, mv, "~/.ssh/"+cr.From, "~/.ssh/"+cr.To)
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return res, err
	}
	type step struct {
		mv        Move
		src, dest string
	}
	var steps []step
	taken := map[string]bool{}
	for _, mv := range moves {
		switch mv.Kind {
		case MoveMisplaced:
			ref := manifest.KeyRef{Profile: mv.Profile, KeyName: mv.Name}
			if _, ok := m.KeySpecFor(ref); !ok {
				return res, fmt.Errorf("%s: profile %s declares no key %s", mv.Line(), mv.Profile, mv.Name)
			}
			dest := m.KeyPath(p.SSHDir, ref)
			src := findMisplaced(p, m, ref)
			if src == "" {
				return res, fmt.Errorf("%s: no copy of it outside %s", mv.Line(), relSSH(p, filepath.Dir(dest)))
			}
			steps = append(steps, step{mv, src, dest})
		case MoveKey:
			if err := checkTarget(m, mv); err != nil {
				return res, err
			}
			src, dest := srcAbs(p, mv.Src), Dest(p, m, mv)
			if mv.FromConfigHome() {
				steps = append(steps, step{mv, "", dest})
				continue
			}
			if err := checkPair(src); err != nil {
				return res, fmt.Errorf("%s: %w", mv.Line(), err)
			}
			if src != dest {
				for _, f := range []string{dest, dest + ".pub"} {
					if _, err := os.Lstat(f); err == nil {
						return res, fmt.Errorf("%s: ~/.ssh/%s is already there", mv.Line(), relSSH(p, f))
					}
				}
			}
			steps = append(steps, step{mv, src, dest})
		}
	}
	for _, s := range steps {
		if err := checkDestDir(p, s.dest); err != nil {
			return res, fmt.Errorf("%s: %w", s.mv.Line(), err)
		}
		if taken[s.dest] {
			return res, fmt.Errorf("two moves put a key at ~/.ssh/%s", relSSH(p, s.dest))
		}
		taken[s.dest] = true
	}

	backup := BackupDir(p, time.Now())
	for _, s := range steps {
		if s.src != "" && s.src != s.dest {
			if err := backupPair(p, s.src, backup); err != nil {
				return res, fmt.Errorf("backing up %s: %w", s.mv.Line(), err)
			}
			res.Backup = backup
			if err := movePair(s.src, s.dest); err != nil {
				return res, fmt.Errorf("%s: %w", s.mv.Line(), err)
			}
			from := "~/.ssh/" + relSSH(p, s.src)
			res.Moved = append(res.Moved, from+" -> ~/.ssh/"+relSSH(p, s.dest))
			changed, err := rewriteRefs(p, s.src, s.dest)
			if err != nil {
				return res, err
			}
			res.Rewrote = append(res.Rewrote, changed...)
			audit(p, s.mv, from, "~/.ssh/"+relSSH(p, s.dest))
		}
		if s.mv.Kind == MoveKey {
			prof := m.Profiles[s.mv.Profile]
			prof.Keys = append(prof.Keys, manifest.KeySpec{Name: s.mv.Name})
			m.SetProfile(s.mv.Profile, prof)
			res.Declared = append(res.Declared, s.mv.Profile+"/"+s.mv.Name)
			if s.src == s.dest {
				audit(p, s.mv, "~/.ssh/"+relSSH(p, s.src), "~/.ssh/"+relSSH(p, s.dest))
			}
		}
	}
	if len(res.Declared) > 0 {
		if err := m.Validate(); err != nil {
			return res, err
		}
		if err := m.Save(p.Manifest()); err != nil {
			return res, err
		}
		inv, err := inventory.Load(p.Inventory())
		if err != nil {
			return res, err
		}
		r := reconciler.New(p, m, inv, platform.EmitUseKeychain())
		for _, d := range res.Declared {
			prof, key, _ := strings.Cut(d, "/")
			if _, err := r.RecordExisting(manifest.KeyRef{Profile: prof, KeyName: key}); err != nil {
				return res, err
			}
		}
	}
	for _, ln := range res.Moved {
		_, _ = fmt.Fprintf(out, "moved %s\n", ln)
	}
	for _, d := range res.Declared {
		_, _ = fmt.Fprintf(out, "declared %s\n", d)
	}
	for _, f := range res.Rewrote {
		_, _ = fmt.Fprintf(out, "repointed IdentityFile lines in %s\n", f)
	}
	return res, nil
}

// MoveConfigHome moves the config-home keys a plan names into the profile
// directories the manifest under p resolves, backed up first. It runs on
// apply only, before Apply declares them.
func MoveConfigHome(p paths.Paths, moves []Move, out io.Writer) error {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	backup := BackupDir(p, time.Now())
	for _, mv := range moves {
		if !mv.FromConfigHome() {
			continue
		}
		if err := checkTarget(m, mv); err != nil {
			return err
		}
		src, dest := srcAbs(p, mv.Src), Dest(p, m, mv)
		if err := checkPair(src); err != nil {
			return fmt.Errorf("%s: %w", mv.Line(), err)
		}
		for _, f := range []string{dest, dest + ".pub"} {
			if _, err := os.Lstat(f); err == nil {
				return fmt.Errorf("%s: ~/.ssh/%s is already there", mv.Line(), relSSH(p, f))
			}
		}
		if err := checkDestDir(p, dest); err != nil {
			return fmt.Errorf("%s: %w", mv.Line(), err)
		}
		if err := backupPair(p, src, backup); err != nil {
			return err
		}
		if err := movePair(src, dest); err != nil {
			return fmt.Errorf("%s: %w", mv.Line(), err)
		}
		to := "~/.ssh/" + relSSH(p, dest)
		_, _ = fmt.Fprintf(out, "moved %s -> %s\n", mv.Src, to)
		audit(p, mv, mv.Src, to)
	}
	return nil
}

// ConfigHomeEffects describes what MoveConfigHome would do, for a preview.
func ConfigHomeEffects(p paths.Paths, moves []Move) []string {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil
	}
	var out []string
	for _, mv := range moves {
		if mv.FromConfigHome() {
			out = append(out, "moves "+mv.Src+" (and its .pub) to ~/.ssh/"+relSSH(p, Dest(p, m, mv)))
		}
	}
	return out
}

// checkTarget refuses a key move into a profile that cannot take it.
func checkTarget(m *manifest.Manifest, mv Move) error {
	prof, ok := m.Profiles[mv.Profile]
	if !ok {
		return fmt.Errorf("%s: unknown profile %q", mv.Line(), mv.Profile)
	}
	if prof.InOnePassword() {
		return fmt.Errorf("%s: profile %s keeps its keys in 1Password, not on disk", mv.Line(), mv.Profile)
	}
	for _, k := range prof.Keys {
		if k.Name == mv.Name {
			return fmt.Errorf("%s: profile %s already declares key %s", mv.Line(), mv.Profile, mv.Name)
		}
	}
	return nil
}

func audit(p paths.Paths, mv Move, from, to string) {
	log.Audit(p.AuditLog(), "structure.move",
		log.Field{Key: "plan", Value: mv.Line()},
		log.Field{Key: "from", Value: from}, log.Field{Key: "to", Value: to})
}

// findMisplaced is where a declared key sits when not where the manifest
// resolves it: the first profile directory of the same name under the key
// root holding it.
func findMisplaced(p paths.Paths, m *manifest.Manifest, ref manifest.KeyRef) string {
	want := filepath.Dir(m.KeyPath(p.SSHDir, ref))
	for _, f := range manifest.GlobKeyTree(p.SSHDir, manifest.KeyRoot(m), ref.KeyName) {
		if filepath.Base(filepath.Dir(f)) == ref.Profile && filepath.Dir(f) != want {
			if fi, err := os.Lstat(f); err == nil && fi.Mode().IsRegular() {
				return f
			}
		}
	}
	return ""
}

// checkPair wants a regular private key file (not a symlink); its .pub, when
// there is one, must be a regular file too.
func checkPair(priv string) error {
	fi, err := os.Lstat(priv)
	if err != nil {
		return fmt.Errorf("no key at %s", priv)
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file (a symlink is never moved)", priv)
	}
	if fi, err := os.Lstat(priv + ".pub"); err == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s.pub is not a regular file", priv)
	}
	return nil
}

// backupPair copies the pair at src (owner-only) into dir, under its path
// relative to its home, before it moves.
func backupPair(p paths.Paths, src, dir string) error {
	rel := relSSH(p, src)
	if strings.HasPrefix(rel, "..") {
		rel = filepath.Join("config", filepath.Base(src))
	} else {
		rel = filepath.Join("ssh", filepath.FromSlash(rel))
	}
	for _, f := range []string{"", ".pub"} {
		b, err := os.ReadFile(src + f)
		if err != nil {
			if f == ".pub" && os.IsNotExist(err) {
				continue
			}
			return err
		}
		dst := filepath.Join(dir, rel+f)
		if err := os.MkdirAll(filepath.Dir(dst), perms.DirMode); err != nil {
			return err
		}
		if err := fs.WriteTextAtomic(dst, string(b), perms.PrivateKeyMode); err != nil {
			return err
		}
	}
	return nil
}

// movePair moves a private key and its .pub (and -cert.pub) to dest, in
// profile directories made owner-only; the moved files get the modes ssh
// wants. A rename across file systems falls back to copy and remove.
// checkDestDir refuses a destination whose directory, or any directory
// between it and ~/.ssh, is a symlink: creating and chmodding it would land
// on the link's target, outside ~/.ssh (as the importer refuses).
func checkDestDir(p paths.Paths, dest string) error {
	root := filepath.Clean(p.SSHDir)
	for d := filepath.Dir(dest); len(d) > len(root) && strings.HasPrefix(d, root+string(filepath.Separator)); d = filepath.Dir(d) {
		fi, err := os.Lstat(d)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("~/.ssh/%s is a symlink; a key is never moved through one", relSSH(p, d))
		}
	}
	return nil
}

func movePair(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), perms.DirMode); err != nil {
		return err
	}
	for _, f := range []string{"", ".pub", "-cert.pub"} {
		if _, err := os.Lstat(src + f); err != nil {
			continue
		}
		if err := rename(src+f, dest+f); err != nil {
			return err
		}
		mode := perms.PublicKeyMode
		if f == "" {
			mode = perms.PrivateKeyMode
		}
		if err := os.Chmod(dest+f, mode); err != nil {
			return err
		}
	}
	return nil
}

func rename(from, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	fi, err := os.Stat(from)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Chtimes(to, fi.ModTime(), fi.ModTime())
	return os.Remove(from)
}
