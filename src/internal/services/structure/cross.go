package structure

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/reconciler"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// crossFile is one key pair a cross-profile move carries to its new profile.
type crossFile struct {
	mv        Move
	from, to  manifest.KeyRef
	src, dest string
}

// applyCross carries out the plan's moves between profiles: keys first
// (MoveDeclared), then hosts (MoveHost). A key moves with its file pair, its
// declaration and its inventory record (deployments included: it is the same
// key), and it may move only when every host of its profile that uses it
// moves to the same profile in the same plan, so no host is left pointing at
// a key that is gone. A host moves with its block; it keeps the key it had
// when that key moves with it - named in this plan, or its own per-host key
// that nothing else uses, which is then carried along - and otherwise takes
// the key its new profile resolves for it (`ssh reconcile` mints it when it
// is not on disk yet, and it must be deployed before the host logs in with
// it). Keys kept where they are (`ssh key adopt`) and 1Password profiles
// never move. Everything is checked before anything is touched; each pair is
// backed up first, as a stray key's move is.
func applyCross(p paths.Paths, moves []Move, out io.Writer, res *Result) error {
	var keyMoves, hostMoves []Move
	for _, mv := range moves {
		switch mv.Kind {
		case MoveDeclared:
			keyMoves = append(keyMoves, mv)
		case MoveHost:
			hostMoves = append(hostMoves, mv)
		}
	}
	if len(keyMoves)+len(hostMoves) == 0 {
		return nil
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	profiles := func(mv Move) (manifest.Profile, manifest.Profile, error) {
		from, ok := m.Profiles[mv.From]
		if !ok {
			return from, from, fmt.Errorf("%s: unknown profile %q", mv.Line(), mv.From)
		}
		to, ok := m.Profiles[mv.Profile]
		if !ok {
			return from, to, fmt.Errorf("%s: unknown profile %q", mv.Line(), mv.Profile)
		}
		if mv.From == mv.Profile {
			return from, to, fmt.Errorf("%s: it is already in profile %s", mv.Line(), mv.Profile)
		}
		if from.InOnePassword() || to.InOnePassword() {
			return from, to, fmt.Errorf("%s: a 1Password profile's keys are not on disk to move", mv.Line())
		}
		return from, to, nil
	}
	// Hosts moving, by from/alias, to the profile each goes to.
	hostTo := map[string]string{}
	for _, mv := range hostMoves {
		hostTo[mv.From+"/"+mv.Name] = mv.Profile
	}
	refs, err := m.KeyRefs()
	if err != nil {
		return err
	}
	var files []crossFile
	renamed := map[manifest.KeyRef]manifest.KeyRef{}
	for _, mv := range keyMoves {
		fromP, toP, err := profiles(mv)
		if err != nil {
			return err
		}
		from := manifest.KeyRef{Profile: mv.From, KeyName: mv.Key}
		to := manifest.KeyRef{Profile: mv.Profile, KeyName: mv.Name}
		if !slices.Contains(refs, from) {
			return fmt.Errorf("%s: profile %s has no key %s", mv.Line(), mv.From, mv.Key)
		}
		if manifest.AdoptedPath(m, from) != "" {
			return fmt.Errorf("%s: %s is kept where it is (`ssh key adopt`), so it never moves", mv.Line(), from)
		}
		if fromP.KeyScope == "shared" && fromP.KeyName != nil && *fromP.KeyName == mv.Key {
			return fmt.Errorf("%s: it is profile %s's shared key; give the profile another first", mv.Line(), mv.From)
		}
		if _, ok := m.KeySpecFor(to); ok || slices.Contains(refs, to) {
			return fmt.Errorf("%s: profile %s already has a key %s", mv.Line(), mv.Profile, mv.Name)
		}
		if toP.KeyScope == "shared" {
			return fmt.Errorf("%s: profile %s uses one shared key for every host, so a second key there would serve none", mv.Line(), mv.Profile)
		}
		hosts, err := m.HostsForKey(from)
		if err != nil {
			return err
		}
		for _, h := range hosts {
			if hostTo[mv.From+"/"+h.Alias] != mv.Profile {
				return fmt.Errorf("%s: host %s still uses it in profile %s; move it too (host %s/%s profile %s)",
					mv.Line(), h.Alias, mv.From, mv.From, h.Alias, mv.Profile)
			}
		}
		renamed[from] = to
		files = append(files, crossFile{mv: mv, from: from, to: to})
	}
	var notes []string
	for _, mv := range hostMoves {
		fromP, toP, err := profiles(mv)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(fromP.Hosts, func(h manifest.Host) bool { return h.Alias == mv.Name })
		if i < 0 {
			return fmt.Errorf("%s: profile %s has no host %s", mv.Line(), mv.From, mv.Name)
		}
		h := fromP.Hosts[i]
		oldKey, err := m.ResolvedKeyName(mv.From, h)
		if err != nil {
			return fmt.Errorf("%s: %w", mv.Line(), err)
		}
		from := manifest.KeyRef{Profile: mv.From, KeyName: oldKey}
		switch to, ok := renamed[from]; {
		case ok:
			h.KeyName = &to.KeyName
		case carriesOwnKey(m, p, mv, h, from, hostTo, refs):
			to := manifest.KeyRef{Profile: mv.Profile, KeyName: oldKey}
			renamed[from] = to
			name := oldKey
			h.KeyName = &name
			files = append(files, crossFile{mv: Move{Kind: MoveDeclared, From: mv.From, Key: oldKey, Profile: mv.Profile, Name: oldKey}, from: from, to: to})
		}
		fromP.Hosts = slices.Delete(slices.Clone(fromP.Hosts), i, i+1)
		toP.Hosts = append(slices.Clone(toP.Hosts), h)
		m.SetProfile(mv.From, fromP)
		m.SetProfile(mv.Profile, toP)
		if _, ok := renamed[from]; !ok {
			if k, err := m.ResolvedKeyName(mv.Profile, h); err == nil {
				notes = append(notes, fmt.Sprintf("host %s now uses key %s/%s; if it is not on disk, `ssh reconcile` mints it, and deploy it before the host logs in with it", h.Alias, mv.Profile, k))
			}
		}
	}
	// The key declarations follow their keys.
	for _, f := range files {
		fromP, toP := m.Profiles[f.from.Profile], m.Profiles[f.to.Profile]
		spec := manifest.KeySpec{Name: f.to.KeyName}
		if j := slices.IndexFunc(fromP.Keys, func(k manifest.KeySpec) bool { return k.Name == f.from.KeyName }); j >= 0 {
			spec = fromP.Keys[j]
			spec.Name = f.to.KeyName
			fromP.Keys = slices.Delete(slices.Clone(fromP.Keys), j, j+1)
			m.SetProfile(f.from.Profile, fromP)
		}
		toP.Keys = append(slices.Clone(toP.Keys), spec)
		m.SetProfile(f.to.Profile, toP)
	}
	if err := m.Validate(); err != nil {
		return err
	}
	// Where each pair is and goes, every one checked before any moves.
	old, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	taken := map[string]bool{}
	for i := range files {
		f := &files[i]
		f.src = inside(p.SSHDir, old.KeyPath(p.SSHDir, f.from))
		f.dest = inside(p.SSHDir, m.KeyPath(p.SSHDir, f.to))
		if f.src == "" || f.dest == "" {
			return fmt.Errorf("%s: a path outside ~/.ssh", f.mv.Line())
		}
		if taken[f.dest] {
			return fmt.Errorf("two moves put a key at ~/.ssh/%s", relSSH(p, f.dest))
		}
		taken[f.dest] = true
		if _, err := os.Lstat(f.src); os.IsNotExist(err) {
			f.src = "" // not minted yet: only its declaration moves
			continue
		}
		if err := checkPair(f.src); err != nil {
			return fmt.Errorf("%s: %w", f.mv.Line(), err)
		}
		for _, x := range []string{f.dest, f.dest + ".pub"} {
			if _, err := os.Lstat(x); err == nil {
				return fmt.Errorf("%s: ~/.ssh/%s is already there", f.mv.Line(), relSSH(p, x))
			}
		}
		if err := checkDestDir(p, f.dest); err != nil {
			return fmt.Errorf("%s: %w", f.mv.Line(), err)
		}
	}

	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		return err
	}
	backup := BackupDir(p, time.Now())
	for _, f := range files {
		if f.src == "" {
			continue
		}
		if err := backupPair(p, f.src, backup); err != nil {
			return fmt.Errorf("backing up %s: %w", f.mv.Line(), err)
		}
		res.Backup = backup
		if err := movePair(f.src, f.dest); err != nil {
			return fmt.Errorf("%s: %w", f.mv.Line(), err)
		}
		fromRel, toRel := "~/.ssh/"+relSSH(p, f.src), "~/.ssh/"+relSSH(p, f.dest)
		res.Moved = append(res.Moved, fromRel+" -> "+toRel)
		changed, err := rewriteRefs(p, f.src, f.dest)
		if err != nil {
			return err
		}
		res.Rewrote = append(res.Rewrote, changed...)
		// The same key, so the same record - deployments and all - at its
		// new path and profile.
		oldIdent, newIdent := old.IdentityFile(f.from.Profile, f.from.KeyName), m.IdentityFile(f.to.Profile, f.to.KeyName)
		for fp, rec := range inv.Keys {
			if rec.Path == oldIdent {
				rec.Path, rec.Profile = newIdent, f.to.Profile
				inv.Keys[fp] = rec
			}
		}
		audit(p, f.mv, fromRel, toRel)
	}
	for _, mv := range hostMoves {
		audit(p, mv, "profile "+mv.From, "profile "+mv.Profile)
	}
	if err := m.Save(p.Manifest()); err != nil {
		return err
	}
	if err := inv.Save(p.Inventory()); err != nil {
		return err
	}
	r := reconciler.New(p, m, inv, platform.EmitUseKeychain())
	for _, f := range files {
		if f.src != "" {
			if _, err := r.RecordExisting(f.to); err != nil {
				return err
			}
		}
		_, _ = fmt.Fprintf(out, "key %s -> %s\n", f.from, f.to)
	}
	for _, mv := range hostMoves {
		_, _ = fmt.Fprintf(out, "host %s: profile %s -> %s\n", mv.Name, mv.From, mv.Profile)
	}
	for _, n := range notes {
		_, _ = fmt.Fprintln(out, n)
	}
	return nil
}

// carriesOwnKey reports whether a host moving to another profile takes its
// key along: a per-host key on disk, used by no host staying behind, not kept
// where it is, and free in the new profile, which gives each host its own key.
func carriesOwnKey(m *manifest.Manifest, p paths.Paths, mv Move, h manifest.Host, from manifest.KeyRef,
	hostTo map[string]string, refs []manifest.KeyRef) bool {
	fromP, toP := m.Profiles[mv.From], m.Profiles[mv.Profile]
	if fromP.KeyScope == "shared" || toP.KeyScope == "shared" || manifest.AdoptedPath(m, from) != "" {
		return false
	}
	if slices.Contains(refs, manifest.KeyRef{Profile: mv.Profile, KeyName: from.KeyName}) || keyDeclared(m, mv.Profile, from.KeyName) {
		return false
	}
	hosts, err := m.HostsForKey(from)
	if err != nil {
		return false
	}
	for _, o := range hosts {
		if o.Alias != h.Alias && hostTo[mv.From+"/"+o.Alias] != mv.Profile {
			return false
		}
	}
	if len(hosts) > 1 {
		return false // several hosts share it: a key move line says so explicitly
	}
	_, err = os.Lstat(filepath.Join(m.ProfileDir(p.SSHDir, mv.From), fs.Segment(from.KeyName)))
	return err == nil
}

func keyDeclared(m *manifest.Manifest, profile, key string) bool {
	_, ok := m.KeySpecFor(manifest.KeyRef{Profile: profile, KeyName: key})
	return ok
}

// Profiles names the profiles a move touches: the one it moves to, and for a
// move between profiles the one it leaves.
func (mv Move) Profiles() []string {
	if mv.Kind == MoveHost || mv.Kind == MoveDeclared {
		return []string{mv.From, mv.Profile}
	}
	return []string{mv.Profile}
}
