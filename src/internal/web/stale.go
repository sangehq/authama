package web

import (
	"os"
	"path/filepath"
	"time"
)

// binaryWatch notices that the program serving the UI was rebuilt or
// reinstalled after it started. A running server keeps the code it started
// with, so after `make install` the page would still show the old UI with
// nothing to say why; the layout shows a banner instead.
type binaryWatch struct {
	path    string
	started time.Time
}

// watchOwnBinary watches the running executable, from now.
func watchOwnBinary(now time.Time) binaryWatch {
	exe, err := os.Executable()
	if err != nil {
		return binaryWatch{}
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return binaryWatch{path: exe, started: now}
}

// replaced is the file at the executable's path when it changed after the
// server started (rebuilt, reinstalled or removed), or "" when it did not.
func (b binaryWatch) replaced() string {
	if b.path == "" {
		return ""
	}
	fi, err := os.Stat(b.path)
	if err != nil || fi.ModTime().After(b.started) {
		return b.path
	}
	return ""
}
