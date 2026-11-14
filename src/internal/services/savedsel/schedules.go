package savedsel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sangehq/authama/src/internal/util/encode"
	fsutil "github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Schedule writes a saved selection to a folder each day, in one format,
// as <selection>.<extension>. Schedules are this account's alone: a folder
// is a path on this machine, so they never go in the manifest.
type Schedule struct {
	Selection string `json:"selection"`
	Format    string `json:"format"`
	Folder    string `json:"folder"`
}

// scheduleFile is schedules.json.
type scheduleFile struct {
	Version   int        `json:"version"`
	Schedules []Schedule `json:"schedules"`
}

// SchedulesFile is where this account's export schedules are kept.
func SchedulesFile(p paths.Paths) string {
	return filepath.Join(p.State(), "exports", "schedules.json")
}

// Schedules are this account's export schedules, in the order added.
func Schedules(p paths.Paths) ([]Schedule, error) {
	b, err := os.ReadFile(SchedulesFile(p))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f scheduleFile
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("%s: %w", SchedulesFile(p), err)
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("%s: version %d, this authama reads version 1", SchedulesFile(p), f.Version)
	}
	return f.Schedules, nil
}

func writeSchedules(p paths.Paths, all []Schedule) error {
	if all == nil {
		all = []Schedule{}
	}
	b, err := json.MarshalIndent(scheduleFile{Version: 1, Schedules: all}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(SchedulesFile(p)), 0o700); err != nil {
		return err
	}
	return fsutil.WriteTextAtomic(SchedulesFile(p), string(b)+"\n", 0o600)
}

// CheckSchedule says why s cannot be scheduled, or nil: a selection name, a
// table format (the SSH client formats write a host list another way), and
// an absolute folder.
func CheckSchedule(s Schedule) error {
	if err := CheckName(s.Selection); err != nil {
		return err
	}
	f, err := encode.Get(s.Format)
	if err != nil {
		return err
	}
	if f.IsHosts() {
		return fmt.Errorf("%s writes hosts for an SSH client; a schedule writes one of %s", s.Format, strings.Join(encode.TableNames(), ", "))
	}
	if !filepath.IsAbs(s.Folder) {
		return fmt.Errorf("the folder %q is not an absolute path", s.Folder)
	}
	return nil
}

// File is where s writes.
func (s Schedule) File() (string, error) {
	f, err := encode.Get(s.Format)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.Folder, s.Selection+"."+f.Extension), nil
}

// AddSchedule keeps s, replacing a schedule of the same selection and
// format (so the folder can be changed by adding it again).
func AddSchedule(p paths.Paths, s Schedule) error {
	if err := CheckSchedule(s); err != nil {
		return err
	}
	all, err := Schedules(p)
	if err != nil {
		return err
	}
	all = slices.DeleteFunc(all, func(x Schedule) bool { return x.Selection == s.Selection && x.Format == s.Format })
	return writeSchedules(p, append(all, s))
}

// RemoveSchedules removes the schedules of selection: in format, or in every
// format when format is "". It answers how many went.
func RemoveSchedules(p paths.Paths, selection, format string) (int, error) {
	all, err := Schedules(p)
	if err != nil {
		return 0, err
	}
	n := len(all)
	all = slices.DeleteFunc(all, func(x Schedule) bool { return x.Selection == selection && (format == "" || x.Format == format) })
	if len(all) == n {
		return 0, fmt.Errorf("no schedule for %q%s", selection, map[bool]string{true: " in " + format}[format != ""])
	}
	return n - len(all), writeSchedules(p, all)
}
