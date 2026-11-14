package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// KeyRootResult reports what MoveKeyRoot changed.
type KeyRootResult struct {
	// From and To are the key roots before and after, as directory names
	// under ~/.ssh.
	From, To string
	// Moved is true when ~/.ssh/<From> existed and was renamed to ~/.ssh/<To>.
	Moved bool
	// RecordsUpdated counts the inventory records whose path followed.
	RecordsUpdated int
}

// MoveKeyRoot makes root the directory under ~/.ssh that holds the profile
// directories (defaults.key_root), moving the whole tree there: private keys,
// .pub files, old/ archives and category config files, as one rename. The
// inventory records follow. Adopted keys stay where they are; they never
// lived under the root.
//
// It refuses when both directories exist (merge them by hand first) and when
// the new name is taken by a file. A key root already moved by hand only has
// its records updated. Any failure after the rename puts the tree back. The
// caller re-renders ~/.ssh/config, whose IdentityFile and Include lines name
// the root.
func MoveKeyRoot(e *Editor, root string) (KeyRootResult, error) {
	m, err := e.load()
	if err != nil {
		return KeyRootResult{}, err
	}
	res := KeyRootResult{From: manifest.KeyRoot(m)}
	if root == manifest.DefaultKeyRoot {
		root = ""
	}
	m.Defaults.KeyRoot = root
	if err := m.Validate(); err != nil {
		return KeyRootResult{}, err
	}
	res.To = manifest.KeyRoot(m)
	if res.From == res.To {
		return res, nil
	}

	// A key root is one directory name directly under ~/.ssh (CheckKeyRoot, run
	// by Validate above). Checked again here, where the path is built, so a
	// root that is not one plain name can never reach a rename.
	if !oneName(res.From) || !oneName(res.To) {
		return KeyRootResult{}, fmt.Errorf("a key root is one directory name under ~/.ssh, not %q or %q", res.From, res.To)
	}
	// Every look and rename goes through an os.Root on ~/.ssh, so even a name
	// that got past the checks above cannot reach outside it.
	if err := os.MkdirAll(e.p.SSHDir, perms.DirMode); err != nil {
		return KeyRootResult{}, err
	}
	ssh, err := os.OpenRoot(e.p.SSHDir)
	if err != nil {
		return KeyRootResult{}, err
	}
	defer func() { _ = ssh.Close() }()
	oldName, newName := filepath.Base(res.From), filepath.Base(res.To)
	rootDir := func(name string) bool {
		fi, err := ssh.Lstat(name)
		return err == nil && fi.IsDir()
	}
	if fi, err := ssh.Lstat(newName); err == nil {
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return KeyRootResult{}, fmt.Errorf("~/.ssh/%s exists and is not a directory; move it aside first", res.To)
		}
		if rootDir(oldName) {
			return KeyRootResult{}, fmt.Errorf("both ~/.ssh/%s and ~/.ssh/%s exist - merge them by hand, then rerun",
				res.From, res.To)
		}
	}
	if rootDir(oldName) {
		if err := ssh.Rename(oldName, newName); err != nil {
			return KeyRootResult{}, err
		}
		res.Moved = true
	}
	undo := func() {
		if res.Moved {
			_ = ssh.Rename(newName, oldName)
		}
	}

	inv, err := inventory.Load(e.p.Inventory())
	if err != nil {
		undo()
		return KeyRootResult{}, err
	}
	oldPrefix := "~/.ssh/" + res.From + "/"
	newPrefix := "~/.ssh/" + res.To + "/"
	for fp, rec := range inv.Keys {
		if rest, ok := strings.CutPrefix(rec.Path, oldPrefix); ok {
			rec.Path = newPrefix + rest
			inv.Keys[fp] = rec
			res.RecordsUpdated++
		}
	}
	if err := m.Save(e.p.Manifest()); err != nil {
		undo()
		return KeyRootResult{}, err
	}
	if res.RecordsUpdated > 0 {
		if err := inv.Save(e.p.Inventory()); err != nil {
			return res, fmt.Errorf("the manifest names ~/.ssh/%s now but the inventory was not updated: %w", res.To, err)
		}
	}
	log.Audit(e.p.AuditLog(), "defaults.key_root",
		log.Field{Key: "from", Value: res.From}, log.Field{Key: "to", Value: res.To},
		log.Field{Key: "moved", Value: res.Moved})
	return res, nil
}

// oneName reports whether name is a single, plain path element: not empty,
// not "." or "..", and without a separator.
func oneName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && filepath.IsLocal(name)
}
