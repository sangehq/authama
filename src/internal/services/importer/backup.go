package importer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sangehq/authama/src/internal/util/homeperms"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// BackupTargets copies manifest + inventory into <home>/.state/import-backup-<ts>/.
// A replacing import runs only after it, so any error is returned: an import
// that went ahead without the copy it promised could not be undone.
func BackupTargets(p paths.Paths) error {
	backup := ""
	for _, src := range []string{p.Manifest(), p.Inventory()} {
		if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if backup == "" {
			dir, err := newBackupDir(p.StateDir(), "import-backup-"+time.Now().Format("20060102-150405"))
			if err != nil {
				return fmt.Errorf("backing up %s before the import: %w", filepath.Base(src), err)
			}
			backup = dir
		}
		if err := copyToBackup(src, filepath.Join(backup, filepath.Base(src))); err != nil {
			return fmt.Errorf("backing up %s before the import: %w", filepath.Base(src), err)
		}
	}
	return nil
}

// newBackupDir makes a directory of its own under parent: name, or name-2,
// name-3, ... when an earlier backup has it. It is created exclusively, so two
// imports in the same second never share one and overwrite each other's copy.
func newBackupDir(parent, name string) (string, error) {
	if err := os.MkdirAll(parent, homeperms.DirMode); err != nil {
		return "", err
	}
	dir := filepath.Join(parent, name)
	for n := 2; ; n++ {
		err := os.Mkdir(dir, homeperms.DirMode)
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		dir = filepath.Join(parent, fmt.Sprintf("%s-%d", name, n))
	}
}

func copyToBackup(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, homeperms.FileMode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
