// Package migratesvc brings a home from an earlier release to the current
// layout.
//
// The config home moved twice: ~/.sshmgr and ~/.config/sshmgr (sshmgr), then
// ~/.config/ssh-manager (%APPDATA%\ssh-manager on Windows; ssh-manager and
// authama 0.1.0), and now ~/.config/authama on every OS. Migration COPIES the
// newest legacy home into the current one and leaves the old folder where it
// was, with a MIGRATED-TO-AUTHAMA.txt marker in it, so an older binary still
// installed beside this one keeps working and nothing is lost if the copy is
// not what the user wanted. It runs once on its own (AutoMigrate, the first
// time authama starts with no home), and `authama migrate` is the guided path
// for when both homes already exist.
//
// The tool's own state - log/, snapshots/, dist/ and .state/ - then moves out
// of the config home into the state home (RelocateState) when those differ.
package migratesvc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/util/homeperms"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// Result is the outcome of a migrate run.
type Result struct {
	Moved   bool
	Legacy  string
	Home    string
	Backup  string
	Message string
	// Relocated lists the state folders moved to the state home.
	Relocated []string
}

// Format renders the message.
func (r Result) Format() string { return r.Message }

// Migrate copies the newest legacy home into the current home. If the current
// home is absent, the legacy one is copied in. If both exist, it refuses
// unless force - with force the current home is moved aside (using stamp) and
// replaced by the copy. The legacy folder is kept and marked either way.
func Migrate(p paths.Paths, force bool, stamp string) (Result, error) {
	home := p.ConfigDir
	legacy := p.FirstLegacyHome()
	if legacy == "" {
		return Result{Home: home, Message: fmt.Sprintf("no legacy home to migrate (home: %s)", home)}, nil
	}
	if err := os.MkdirAll(filepath.Dir(home), 0o700); err != nil {
		return Result{}, err
	}
	backup := ""
	if exists(home) {
		if !force {
			return Result{}, fmt.Errorf("both %s and %s exist - inspect them, then re-run "+
				"`authama migrate --force` to move the current home aside and replace it with a copy of the legacy one",
				legacy, home)
		}
		backup = filepath.Join(filepath.Dir(home), filepath.Base(home)+".replaced-"+stamp)
		if err := os.Rename(home, backup); err != nil {
			return Result{}, err
		}
	}
	res, err := copyIn(p, legacy)
	if err != nil {
		return Result{}, err
	}
	res.Backup = backup
	if backup != "" {
		res.Message += "; previous home moved to " + backup
	}
	return res, nil
}

// AutoMigrate is the first-run migration: when the current home does not
// exist yet and a legacy one does, copy it in. It does nothing in a sandbox
// or when the home exists, and reports whether it acted.
func AutoMigrate(p paths.Paths) (Result, bool, error) {
	if p.IsDev() || exists(p.ConfigDir) {
		return Result{}, false, nil
	}
	legacy := p.FirstLegacyHome()
	if legacy == "" {
		return Result{}, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(p.ConfigDir), 0o700); err != nil {
		return Result{}, false, err
	}
	res, err := copyIn(p, legacy)
	return res, err == nil, err
}

// copyIn copies legacy to the (absent) home, marks the legacy folder, and
// tightens the copy's modes. A failed copy is removed, so a retry starts
// clean.
func copyIn(p paths.Paths, legacy string) (Result, error) {
	home := p.ConfigDir
	if err := copyTree(legacy, home); err != nil {
		_ = os.RemoveAll(home)
		return Result{}, fmt.Errorf("copying %s to %s: %w", legacy, home, err)
	}
	note := fmt.Sprintf("%s copied this folder to %s on %s.\n"+
		"%s no longer reads it. It is kept so an older release still installed\n"+
		"keeps working and nothing is lost; delete it once you are happy with the\n"+
		"new location.\n", brand.Command, home, time.Now().Format(time.RFC3339), brand.Command)
	if err := os.WriteFile(filepath.Join(legacy, paths.MigratedMarker), []byte(note), 0o600); err != nil {
		return Result{}, fmt.Errorf("copied %s to %s, but could not mark the old folder: %w", legacy, home, err)
	}
	tighten(p)
	return Result{Moved: true, Legacy: legacy, Home: home,
		Message: fmt.Sprintf("copied %s -> %s (the old folder is kept, marked %s)", legacy, home, paths.MigratedMarker)}, nil
}

// RelocateState moves the state folders a config home from before 0.1.1 holds
// (log/, snapshots/, dist/, .state/) into a separate state home. A folder
// already present in the state home is left where it is and reported, never
// merged. It does nothing when the state home is the config home.
func RelocateState(p paths.Paths) (moved, conflicts []string, err error) {
	if !paths.SeparateState(p) || p.IsDev() {
		return nil, nil, nil
	}
	for _, sub := range paths.StateSubdirs {
		src := filepath.Join(p.ConfigDir, sub)
		fi, statErr := os.Lstat(src)
		if statErr != nil || !fi.IsDir() {
			continue
		}
		dst := filepath.Join(p.State(), sub)
		if exists(dst) {
			conflicts = append(conflicts, src)
			continue
		}
		if err := os.MkdirAll(p.State(), 0o700); err != nil {
			return moved, conflicts, err
		}
		if err := moveDir(src, dst); err != nil {
			return moved, conflicts, fmt.Errorf("moving %s to %s: %w", src, dst, err)
		}
		moved = append(moved, sub)
	}
	if len(moved) > 0 {
		tighten(p)
	}
	return moved, conflicts, nil
}

// tighten re-asserts the canonical modes on the freshly migrated home.
//
// Both move paths preserve the source's modes - os.Rename by definition, and
// copyTree explicitly - so a legacy home created under a permissive umask, or one
// that predates this tool enforcing anything, arrived at the new location still
// group- and world-readable. Migrating is exactly the moment to fix that, since
// the user has no reason to suspect the old modes came along.
func tighten(p paths.Paths) {
	for _, mp := range homeperms.SecretPerms(p) {
		if exists(mp.Path) {
			_ = perms.SetPerms(mp.Path, mp.Mode)
		}
	}
}

// moveDir renames src to dst, falling back to a copy+remove across filesystems
// (the caller guarantees dst is absent). Mirrors facade._move_dir.
func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		// The lock belongs to the process holding it in the old home, and a
		// marker from an earlier migration describes that folder, not this one.
		if rel == filepath.Join(".state", ".lock") || rel == paths.MigratedMarker {
			return nil
		}
		if fi.IsDir() {
			return os.MkdirAll(target, fi.Mode().Perm())
		}
		// Recreate a symlink as a symlink. filepath.Walk lstats, so a link arrives
		// here as a non-directory; opening it would follow it and write the
		// target's bytes into a regular file with the link's own 0777 mode.
		// Symlinking a config file out to a dotfiles repo or a password store is a
		// normal thing to do, and a migration that quietly replaced the link with a
		// stale copy would leave the user editing a file nothing reads.
		if fi.Mode()&os.ModeSymlink != 0 {
			dest, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(dest, target)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
