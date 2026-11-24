package sshcfg

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/authkeys"
)

// KeyPair is a private key and its .pub, found by walking ~/.ssh. The
// private path is never read; a pair is recognised by a sibling .pub that
// parses as a public key, or by a private file that has a .pub sibling.
type KeyPair struct {
	Private string // absolute path of the private half (the stem)
	Public  string // absolute path of the .pub, if it exists
	Rel     string // slash-separated path relative to the walk root
}

// skipDirNames are directories under ~/.ssh that are never keys: the agent
// socket dir, ControlPath dir authama uses, a sockets/ folder, and junk
// IDE metadata. Names are compared case-sensitively as they appear on disk.
var skipDirNames = map[string]bool{
	"agent": true, "authama-cm": true, "sockets": true, ".idea": true, ".git": true,
}

// skipFileNames are files that sit in ~/.ssh and are never a key.
var skipFileNames = map[string]bool{
	"config": true, "authorized_keys": true, "authorized_keys2": true,
	"known_hosts": true, "known_hosts.old": true, "known_hosts2": true,
	"environment": true, "rc": true, ".ds_store": true,
}

// SkipPath reports whether a file or directory under ~/.ssh should be
// ignored when looking for keys: known_hosts*, authorized_keys*, sockets,
// ControlPath directories, agent directories, config files, and dotfiles.
func SkipPath(name string, d fs.DirEntry) bool {
	base := name
	if d != nil {
		base = d.Name()
	}
	lower := strings.ToLower(base)
	if strings.HasPrefix(base, ".") && lower != ".pub" {
		// Dot directories and files (.DS_Store, .idea) are not keys.
		if d != nil && d.IsDir() {
			return true
		}
		if lower != ".pub" && !strings.HasSuffix(lower, ".pub") {
			return true
		}
	}
	if skipFileNames[lower] || skipFileNames[base] {
		return true
	}
	if strings.HasPrefix(lower, "known_hosts") || strings.HasPrefix(lower, "authorized_keys") {
		return true
	}
	if strings.HasSuffix(lower, ".sock") || strings.HasPrefix(lower, "agent-") {
		return true
	}
	if d != nil && d.IsDir() && (skipDirNames[base] || skipDirNames[lower] || strings.HasPrefix(lower, "agent-")) {
		return true
	}
	if d != nil && !d.IsDir() {
		if lower == "config" || strings.HasSuffix(lower, ".conf") || strings.HasSuffix(lower, ".cfg") {
			return true
		}
	}
	if d != nil {
		if info, err := d.Info(); err == nil && isSocketOrFIFO(info) {
			return true
		}
	}
	return false
}

func isSocketOrFIFO(fi os.FileInfo) bool {
	mode := fi.Mode()
	return mode&os.ModeSocket != 0 || mode&os.ModeNamedPipe != 0
}

// WalkKeys walks root (typically ~/.ssh) for key pairs, following
// directory symlinks without looping, skipping SkipPath names, and never
// reading a private key. skipRoots, if set, are directories that are not
// walked (the managed key root, which another walker already covers).
func WalkKeys(root string, skipRoots ...string) []KeyPair {
	if root == "" {
		return nil
	}
	skip := map[string]bool{}
	for _, s := range skipRoots {
		if s == "" {
			continue
		}
		skip[filepath.Clean(s)] = true
	}
	seen := map[string]bool{} // dev:ino, to follow symlinks without looping
	stems := map[string]KeyPair{}

	var walk func(dir string)
	walk = func(dir string) {
		if skip[filepath.Clean(dir)] {
			return
		}
		id, ok := dirID(dir)
		if !ok || seen[id] {
			return
		}
		seen[id] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			// A symlink to a directory is walked; a symlink to a file is
			// treated as the file, using Lstat so a broken link is skipped.
			if e.Type()&os.ModeSymlink != 0 {
				fi, err := os.Stat(p)
				if err != nil {
					continue
				}
				if fi.IsDir() {
					if SkipPath(e.Name(), e) {
						continue
					}
					walk(p)
					continue
				}
			}
			if e.IsDir() {
				if SkipPath(e.Name(), e) {
					continue
				}
				walk(p)
				continue
			}
			if SkipPath(e.Name(), e) {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".pub") {
				body, err := os.ReadFile(p)
				if err != nil || !authkeys.IsValidPublicKey(strings.TrimSpace(string(body))) {
					continue
				}
				stem := strings.TrimSuffix(p, ".pub")
				kp := stems[stem]
				kp.Private = stem
				kp.Public = p
				stems[stem] = kp
				continue
			}
			if _, err := os.Lstat(p + ".pub"); err == nil {
				stem := p
				kp := stems[stem]
				kp.Private = stem
				if kp.Public == "" {
					kp.Public = p + ".pub"
				}
				stems[stem] = kp
			}
		}
	}
	walk(root)

	out := make([]KeyPair, 0, len(stems))
	for _, kp := range stems {
		if rel, err := filepath.Rel(root, kp.Private); err == nil && !strings.HasPrefix(rel, "..") {
			kp.Rel = filepath.ToSlash(rel)
		} else {
			kp.Rel = filepath.ToSlash(kp.Private)
		}
		out = append(out, kp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

func dirID(path string) (string, bool) {
	if _, err := os.Stat(path); err != nil {
		return "", false
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(r), true
	}
	return filepath.Clean(path), true
}
