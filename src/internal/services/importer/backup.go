package importer

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sangehq/authama/src/internal/util/homeperms"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// BackupTargets copies manifest + inventory into <home>/.state/import-backup-<ts>/.
func BackupTargets(p paths.Paths) {
	backup := filepath.Join(p.StateDir(), "import-backup-"+time.Now().Format("20060102-150405"))
	for _, src := range []string{p.Manifest(), p.Inventory()} {
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.MkdirAll(backup, homeperms.DirMode); err != nil {
			return
		}
		copyToBackup(src, filepath.Join(backup, filepath.Base(src)))
	}
}

func copyToBackup(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, homeperms.FileMode)
	if err != nil {
		return
	}
	_, _ = io.Copy(out, in)
	_ = out.Close()
}
