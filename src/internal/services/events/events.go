// Package events tracks the files authama is responsible for - everything
// under ~/.ssh, the config home, and each tool's credential file - and records
// every change to them, whoever made it: authama, an editor, ssh-keygen run by
// hand, a package manager rewriting its token file.
//
// It never records a file's contents. Each scan keeps, per file, its size,
// mode, modification time and a SHA-256 of its bytes, in an owner-only state
// file; an event names the file, what changed (added, changed, removed, mode)
// and when it was seen. The hash detects a change and cannot be turned back
// into a key or a token.
package events

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	ufs "github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/homeperms"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// The areas a tracked file belongs to.
const (
	AreaSSH    = "ssh"    // under ~/.ssh
	AreaConfig = "config" // the config home: manifest, inventory, settings, .env
	AreaAuth   = "auth"   // a tool's credential file: auth.json, .npmrc, …
)

// The kinds of event.
const (
	KindStarted = "started" // the first scan: tracking began, nothing compared
	KindAdded   = "added"
	KindChanged = "changed" // the contents changed
	KindRemoved = "removed"
	KindMode    = "mode" // only the permissions changed
)

// Event is one change seen by a scan. Time is when the change happened as far
// as the file says - its modification time, for an added or changed file -
// and Seen is the scan that found it; a removal or a mode change, which leaves
// no time behind, has Time equal to Seen.
type Event struct {
	Time   time.Time `json:"time"`
	Seen   time.Time `json:"seen"`
	Kind   string    `json:"kind"`
	Area   string    `json:"area"`
	Path   string    `json:"path"`
	Size   int64     `json:"size,omitempty"`
	Mode   string    `json:"mode,omitempty"`
	Detail string    `json:"detail,omitempty"`
	// Hash chains the log: SHA-256 of the previous line's Hash and this
	// event's other fields. An edited, removed or inserted line breaks the
	// chain from there on (see Verify).
	Hash string `json:"hash,omitempty"`
}

// Tracked is a file, or a directory walked for files, and its area. Skip
// names directories under it that are not walked.
type Tracked struct {
	Path string
	Area string
	Dir  bool
	Skip []string
}

// Roots is what a home tracks: ~/.ssh and the config home, walked, and each
// credential file named in tokenFiles.
func Roots(p paths.Paths, tokenFiles []string) []Tracked {
	// authama's own state - the history, these events, snapshots, bundles, the
	// lock - sits in the config home unless a state home is set apart; it
	// changes on every command, so it is not tracked.
	own := []string{p.LogDir(), p.StateDir(), p.SnapshotsDir(), p.DistDir()}
	out := []Tracked{{Path: p.SSHDir, Area: AreaSSH, Dir: true}, {Path: p.ConfigDir, Area: AreaConfig, Dir: true, Skip: own}}
	seen := map[string]bool{}
	for _, f := range tokenFiles {
		if f == "" || seen[f] || within(f, p.SSHDir) || within(f, p.ConfigDir) {
			continue
		}
		seen[f] = true
		out = append(out, Tracked{Path: f, Area: AreaAuth})
	}
	return out
}

// within reports whether path is dir or under it: such a file is walked with
// its directory already.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// maxHash is the largest file a scan hashes; a bigger one (never a key or a
// token) is compared by size and time only.
const maxHash = 32 << 20

type fileState struct {
	Area    string `json:"area"`
	Size    int64  `json:"size"`
	Mode    uint32 `json:"mode"`
	ModTime int64  `json:"mtime"`
	Sum     string `json:"sum,omitempty"`
}

type state struct {
	Files map[string]fileState `json:"files"`
	// Head is the Hash of the log's last line, so a log cut short (lines
	// removed from its end) is caught too.
	Head string `json:"head,omitempty"`
}

// StateFile and LogFile are where a home keeps the last scan and the events.
func StateFile(p paths.Paths) string { return filepath.Join(p.StateDir(), "files.json") }
func LogFile(p paths.Paths) string   { return filepath.Join(p.LogDir(), "events.jsonl") }

var mu sync.Mutex // one scan at a time in a process

// Scan compares the tracked files with the last scan, appends what changed to
// the events log, and returns it. The first scan records a single "started"
// event and the files it found.
func Scan(p paths.Paths, roots []Tracked, now time.Time) ([]Event, error) {
	mu.Lock()
	defer mu.Unlock()
	current, err := walk(roots)
	if err != nil {
		return nil, err
	}
	prev, head, err := load(StateFile(p))
	if err != nil {
		return nil, err
	}
	var evs []Event
	if prev == nil {
		evs = []Event{{Time: now, Seen: now, Kind: KindStarted, Detail: fmt.Sprintf("tracking %d file(s)", len(current))}}
	} else {
		evs = diff(prev, current, now)
	}
	if len(evs) > 0 {
		last, err := lastHash(LogFile(p))
		if err != nil {
			return nil, err
		}
		if last == "" {
			last = head
		}
		for i := range evs {
			evs[i].Hash = chain(last, evs[i])
			last = evs[i].Hash
		}
		head = last
		if err := appendLog(LogFile(p), evs); err != nil {
			return nil, err
		}
	}
	if err := save(StateFile(p), current, head); err != nil {
		return nil, err
	}
	return evs, nil
}

// Filter narrows List.
type Filter struct {
	Since time.Time
	Area  string
	Kind  string
	Limit int // 0 is no limit
}

// List reads the events log, newest first. A home that has never been scanned
// has none.
func List(p paths.Paths, f Filter) ([]Event, error) {
	fh, err := os.Open(LogFile(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	var out []Event
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue // a torn last line from a crash is skipped, not fatal
		}
		if !f.Since.IsZero() && e.Seen.Before(f.Since) || f.Area != "" && e.Area != f.Area || f.Kind != "" && e.Kind != f.Kind {
			continue
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// Newest first by when the scans saw them, so the log reads in the order
	// it was written; within one scan, by when the files say they changed.
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Seen.Equal(out[j].Seen) {
			return out[i].Seen.After(out[j].Seen)
		}
		return out[i].Time.After(out[j].Time)
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func walk(roots []Tracked) (map[string]fileState, error) {
	out := map[string]fileState{}
	add := func(path, area string, fi fs.FileInfo) {
		st := fileState{Area: area, Size: fi.Size(), Mode: uint32(fi.Mode().Perm()), ModTime: fi.ModTime().UnixNano()}
		if fi.Size() <= maxHash {
			st.Sum = sum(path)
		}
		out[path] = st
	}
	for _, r := range roots {
		if !r.Dir {
			if fi, err := os.Stat(r.Path); err == nil && fi.Mode().IsRegular() {
				add(r.Path, r.Area, fi)
			}
			continue
		}
		err := filepath.WalkDir(r.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				for _, s := range r.Skip {
					if filepath.Clean(path) == filepath.Clean(s) {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil // sockets, symlinks
			}
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			add(path, r.Area, fi)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func sum(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func diff(prev, cur map[string]fileState, now time.Time) []Event {
	var evs []Event
	mode := func(m uint32) string { return fmt.Sprintf("%04o", m) }
	at := func(s fileState) time.Time {
		if s.ModTime == 0 {
			return now
		}
		return time.Unix(0, s.ModTime).UTC()
	}
	for path, c := range cur {
		p, ok := prev[path]
		switch {
		case !ok:
			evs = append(evs, Event{Time: at(c), Seen: now, Kind: KindAdded, Area: c.Area, Path: path, Size: c.Size, Mode: mode(c.Mode)})
		case changed(p, c):
			evs = append(evs, Event{Time: at(c), Seen: now, Kind: KindChanged, Area: c.Area, Path: path, Size: c.Size, Mode: mode(c.Mode),
				Detail: fmt.Sprintf("%d → %d bytes", p.Size, c.Size)})
		case p.Mode != c.Mode:
			evs = append(evs, Event{Time: now, Seen: now, Kind: KindMode, Area: c.Area, Path: path, Size: c.Size, Mode: mode(c.Mode),
				Detail: mode(p.Mode) + " → " + mode(c.Mode)})
		}
	}
	for path, p := range prev {
		if _, ok := cur[path]; !ok {
			evs = append(evs, Event{Time: now, Seen: now, Kind: KindRemoved, Area: p.Area, Path: path})
		}
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Path < evs[j].Path })
	return evs
}

// changed compares contents: by hash when both scans hashed the file, else by
// size and modification time.
func changed(p, c fileState) bool {
	if p.Sum != "" && c.Sum != "" {
		return p.Sum != c.Sum
	}
	return p.Size != c.Size || p.ModTime != c.ModTime
}

func load(path string) (map[string]fileState, string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, "", nil // an unreadable state is a fresh start, not a failure
	}
	if st.Files == nil {
		st.Files = map[string]fileState{}
	}
	return st.Files, st.Head, nil
}

func save(path string, files map[string]fileState, head string) error {
	if err := ufs.EnsureDir(filepath.Dir(path), homeperms.DirMode); err != nil {
		return err
	}
	b, err := json.Marshal(state{Files: files, Head: head})
	if err != nil {
		return err
	}
	return ufs.WriteTextAtomic(path, string(b), homeperms.FileMode)
}

func appendLog(path string, evs []Event) error {
	if len(evs) == 0 {
		return nil
	}
	if err := ufs.EnsureDir(filepath.Dir(path), homeperms.DirMode); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, homeperms.FileMode)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, e := range evs {
		line, err := json.Marshal(e)
		if err != nil {
			_ = f.Close()
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// chain is an event's hash: SHA-256 over the previous hash and the event's
// fields, Hash left out, in their JSON form.
func chain(prev string, e Event) string {
	e.Hash = ""
	b, _ := json.Marshal(e)
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write([]byte{0})
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// lastHash is the Hash of the log's last chained line, or "" for none.
func lastHash(path string) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	last := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Hash != "" {
			last = e.Hash
		}
	}
	return last, sc.Err()
}

// Integrity is what Verify found.
type Integrity struct {
	Lines     int    // lines read
	Chained   int    // lines carrying a hash
	Unchained int    // lines from before the log was chained, at its start
	Broken    int    // the first line (1-based) whose hash does not follow, 0 when none
	Reason    string // why, when Broken or Truncated
	Truncated bool   // the log ends before the head the state recorded
}

// OK reports whether the log is intact.
func (i Integrity) OK() bool { return i.Broken == 0 && !i.Truncated }

// Verify checks the events log's hash chain: every chained line must hash
// from the one before it, and the last must be the head the state recorded.
// Lines written before the log was chained, at its start, are counted, not
// checked. Tamper evidence, not proof: someone who can rewrite the state
// home can rewrite both files, but not one line without the chain showing it.
func Verify(p paths.Paths) (Integrity, error) {
	var in Integrity
	_, head, err := load(StateFile(p))
	if err != nil {
		return in, err
	}
	f, err := os.Open(LogFile(p))
	if errors.Is(err, os.ErrNotExist) {
		if head != "" {
			in.Truncated, in.Reason = true, "the events log is missing, but the state records a head"
		}
		return in, nil
	}
	if err != nil {
		return in, err
	}
	defer func() { _ = f.Close() }()
	prev := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		in.Lines++
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			if in.Broken == 0 {
				in.Broken, in.Reason = in.Lines, "the line is not a valid event"
			}
			continue
		}
		if e.Hash == "" {
			if in.Chained > 0 && in.Broken == 0 {
				in.Broken, in.Reason = in.Lines, "an unchained line after the chain began"
			}
			in.Unchained++
			continue
		}
		in.Chained++
		if in.Broken == 0 && chain(prev, e) != e.Hash {
			in.Broken, in.Reason = in.Lines, "its hash does not follow from the line before (edited, removed or inserted)"
		}
		prev = e.Hash
	}
	if err := sc.Err(); err != nil {
		return in, err
	}
	if in.Broken == 0 && head != "" && prev != head {
		in.Truncated, in.Reason = true, "the log does not end at the head the state recorded (lines removed from its end)"
	}
	return in, nil
}
