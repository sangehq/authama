// Package exportsched writes the export schedules: saved selections written
// to a folder, one file per selection and format, by `export schedule run`,
// the daily job, and the web UI's Run now. The schedules themselves are kept
// by savedsel.
package exportsched

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/dataset"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// InsideDir reports whether path is dir or lies under it, judged by inode:
// each existing ancestor of path is compared with dir by os.SameFile.
func InsideDir(path, dir string) bool {
	want, err := os.Stat(dir)
	if err != nil {
		return false // no such directory yet: nothing there to overwrite
	}
	for cur := filepath.Clean(path); ; {
		if fi, err := os.Stat(cur); err == nil && os.SameFile(fi, want) {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

// Folder makes folder absolute and refuses one inside ~/.ssh, or, in dev
// mode, outside the sandbox.
func Folder(p paths.Paths, folder string) (string, error) {
	abs, err := filepath.Abs(folder)
	if err != nil {
		return "", err
	}
	if InsideDir(abs, p.SSHDir) {
		return "", fmt.Errorf("refusing %s: it is inside %s, which an export must leave as it is", abs, p.SSHDir)
	}
	if !p.Contains(abs) {
		return "", fmt.Errorf("refusing %s in dev mode: it is outside the sandbox under %s", abs, p.DevRoot)
	}
	return abs, nil
}

// Run writes one schedule's file (mode 0600, in a folder made 0700),
// answering where and how many rows. m may be nil.
func Run(p paths.Paths, m *manifest.Manifest, s savedsel.Schedule) (string, int, error) {
	if err := savedsel.CheckSchedule(s); err != nil {
		return "", 0, err
	}
	if _, err := Folder(p, s.Folder); err != nil {
		return "", 0, err
	}
	saved, err := savedsel.Get(p, m, s.Selection)
	if err != nil {
		return "", 0, err
	}
	ds, err := dataset.Build(p, dataset.Options{Parts: selection.Parts(saved.Selection.Grain)})
	if err != nil {
		return "", 0, err
	}
	res, err := selection.Run(ds, saved.Selection)
	if err != nil {
		return "", 0, err
	}
	f, err := encode.Get(s.Format)
	if err != nil {
		return "", 0, err
	}
	var buf bytes.Buffer
	if err := f.Write(&buf, encode.Table{Columns: res.Columns, Rows: res.Rows},
		encode.Options{Title: brand.Command + " " + s.Selection}); err != nil {
		return "", 0, err
	}
	if err := os.MkdirAll(s.Folder, 0o700); err != nil {
		return "", 0, err
	}
	file, err := s.File()
	if err != nil {
		return "", 0, err
	}
	return file, len(res.Rows), fs.WriteTextAtomic(file, buf.String(), 0o600)
}

// Result is one schedule's run.
type Result struct {
	Schedule savedsel.Schedule
	File     string
	Rows     int
	Err      error
}

// RunAll writes every schedule; one that fails does not stop the rest.
func RunAll(p paths.Paths) ([]Result, error) {
	all, err := savedsel.Schedules(p)
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		m = nil // the user's own selections are still read
	}
	out := make([]Result, 0, len(all))
	for _, s := range all {
		file, rows, err := Run(p, m, s)
		out = append(out, Result{Schedule: s, File: file, Rows: rows, Err: err})
	}
	return out, nil
}
