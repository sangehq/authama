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
	built   time.Time // the binary's modification time when the server started
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
	w := binaryWatch{path: exe, started: now}
	if fi, err := os.Stat(exe); err == nil {
		w.built = fi.ModTime()
	}
	return w
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

// buildInfo is the running build, as the header shows it.
type buildInfo struct {
	Version string
	Built   time.Time // the binary's time when the server started; zero when unknown
	Started time.Time
}

// info is the running build: version, the binary's time at start, and when
// the server started.
func (b binaryWatch) info(version string) buildInfo {
	return buildInfo{Version: version, Built: b.built, Started: b.started}
}
