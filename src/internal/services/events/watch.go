package events

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/sangehq/authama/src/internal/core/sshcfg"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// How the Structure page reports the serve file watch.
const (
	ModeWatching = "watching"
	ModePolling  = "polling"
)

// DefaultDebounce is how long serve waits after a filesystem event before
// scanning: short enough to feel prompt, long enough to collapse a burst
// of writes into one scan.
const DefaultDebounce = 1500 * time.Millisecond

// ExtraRoots are trees serve watches in addition to Roots: the user's
// gitconfig and Docker contexts, where a tool-key move may need to be
// reflected. Missing paths are still listed so the watcher can wait on
// an ancestor and add them when they appear.
func ExtraRoots(p paths.Paths) []Tracked {
	home := filepath.Dir(p.SSHDir)
	return []Tracked{
		{Path: filepath.Join(home, ".gitconfig"), Area: AreaConfig},
		{Path: filepath.Join(home, ".config", "git"), Area: AreaConfig, Dir: true},
		{Path: filepath.Join(home, ".docker", "contexts"), Area: AreaAuth, Dir: true},
	}
}

// IncludeRoots are directories outside ~/.ssh that ~/.ssh/config Includes
// (for example ~/.orbstack/ssh). Paths already under ~/.ssh are omitted:
// Roots already walks that tree. Missing paths are still listed so the
// watcher can wait on an ancestor.
func IncludeRoots(p paths.Paths) []Tracked {
	tree, err := sshcfg.LoadUser(p.SSHDir)
	if err != nil || tree == nil {
		return nil
	}
	var out []Tracked
	seen := map[string]bool{}
	for _, d := range tree.Dirs {
		d = filepath.Clean(d)
		if d == "" || seen[d] || within(d, p.SSHDir) {
			continue
		}
		seen[d] = true
		out = append(out, Tracked{Path: d, Area: AreaSSH, Dir: true})
	}
	return out
}

// StartWatch starts an fsnotify watch on roots and calls onChange after
// debounce of quiet. It returns ModeWatching when at least one path could
// be added, or ModePolling when the watcher could not be created or no
// path could be added. The watch runs until ctx ends. onChange must not
// block indefinitely; it is called from the watch loop.
func StartWatch(ctx context.Context, roots []Tracked, debounce time.Duration, onChange func()) string {
	return startWatch(ctx, roots, nil, debounce, onChange)
}

// StartWatchWith is StartWatch, and after every event (and at each debounce)
// refresh is called so a new Include pointing outside ~/.ssh starts being
// watched without a restart.
func StartWatchWith(ctx context.Context, roots []Tracked, refresh func() []Tracked, debounce time.Duration, onChange func()) string {
	return startWatch(ctx, roots, refresh, debounce, onChange)
}

func startWatch(ctx context.Context, roots []Tracked, refresh func() []Tracked, debounce time.Duration, onChange func()) string {
	if onChange == nil || (len(roots) == 0 && refresh == nil) {
		return ModePolling
	}
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return ModePolling
	}
	n := addRoots(w, roots)
	if n == 0 && refresh == nil {
		_ = w.Close()
		return ModePolling
	}
	if n == 0 && refresh != nil {
		// roots may appear after refresh; keep the watcher anyway.
		n += addRoots(w, refresh())
		if n == 0 {
			_ = w.Close()
			return ModePolling
		}
	}
	go runWatch(ctx, w, roots, refresh, debounce, onChange)
	return ModeWatching
}

func runWatch(ctx context.Context, w *fsnotify.Watcher, roots []Tracked, refresh func() []Tracked, debounce time.Duration, onChange func()) {
	defer func() { _ = w.Close() }()
	var mu sync.Mutex
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()
	arm := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(debounce)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Name != "" && ev.Has(fsnotify.Create) {
				mu.Lock()
				addNew(w, ev.Name, roots)
				roots = refreshRoots(w, roots, refresh)
				mu.Unlock()
			}
			arm()
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
			arm()
		case <-timer.C:
			mu.Lock()
			roots = refreshRoots(w, roots, refresh)
			mu.Unlock()
			onChange()
		}
	}
}

// addRoots watches every existing path in roots (directories recursively,
// files as themselves or their parent) and the nearest existing ancestor
// of a path that is not there yet. It returns how many watches were added.
func addRoots(w *fsnotify.Watcher, roots []Tracked) int {
	n := 0
	for _, r := range roots {
		n += addPath(w, r.Path, r.Dir, r.Skip)
	}
	return n
}

func addPath(w *fsnotify.Watcher, path string, dir bool, skip []string) int {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return 0
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return addAncestor(w, path)
	}
	if !dir || !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
		return addOne(w, path)
	}
	return addTree(w, path, skip)
}

func addTree(w *fsnotify.Watcher, root string, skip []string) int {
	n := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if skipped(path, skip) && path != root {
			return filepath.SkipDir
		}
		n += addOne(w, path)
		return nil
	})
	return n
}

func addNew(w *fsnotify.Watcher, path string, roots []Tracked) {
	path = filepath.Clean(path)
	fi, err := os.Lstat(path)
	if err != nil || !fi.IsDir() {
		return
	}
	for _, r := range roots {
		if skipped(path, r.Skip) {
			return
		}
		if r.Dir && (path == filepath.Clean(r.Path) || under(path, r.Path)) {
			addTree(w, path, r.Skip)
			return
		}
		// A pending extra root (git config, docker contexts) just appeared
		// under the ancestor we were watching.
		if path == filepath.Clean(r.Path) || under(r.Path, path) {
			addPath(w, r.Path, r.Dir, r.Skip)
			return
		}
	}
}

func addAncestor(w *fsnotify.Watcher, path string) int {
	dir := filepath.Dir(path)
	for {
		if dir == "" || dir == "." || dir == string(filepath.Separator) {
			return 0
		}
		// A Windows volume root ("C:\\") is not a useful watch: too broad.
		if vol := filepath.VolumeName(dir); vol != "" && filepath.Clean(dir) == vol+string(filepath.Separator) {
			return 0
		}
		if _, err := os.Lstat(dir); err == nil {
			return addOne(w, dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0
		}
		dir = parent
	}
}

func addOne(w *fsnotify.Watcher, path string) int {
	if err := w.Add(path); err != nil {
		return 0
	}
	return 1
}

func skipped(path string, skip []string) bool {
	c := filepath.Clean(path)
	for _, s := range skip {
		if c == filepath.Clean(s) {
			return true
		}
	}
	return false
}

func under(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "."
}

func refreshRoots(w *fsnotify.Watcher, roots []Tracked, refresh func() []Tracked) []Tracked {
	if refresh == nil {
		return roots
	}
	extra := refresh()
	if len(extra) == 0 {
		return roots
	}
	addRoots(w, extra)
	seen := map[string]bool{}
	for _, r := range roots {
		seen[r.Path] = true
	}
	for _, r := range extra {
		if r.Path == "" || seen[r.Path] {
			continue
		}
		seen[r.Path] = true
		roots = append(roots, r)
	}
	return roots
}
