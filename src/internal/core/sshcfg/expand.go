package sshcfg

import (
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
)

// hasMeta reports whether p holds a glob character OpenSSH's Include
// understands (*, ?, or a character class), or a ** segment.
func hasMeta(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

// Expand resolves one Include token the way OpenSSH does for a user config:
// "~" and "~user" are expanded, a relative path is taken from userSSHDir
// (the directory ssh treats as ~/.ssh, not the including file's directory),
// and globs are expanded in lexical order. A path segment of ** matches
// every directory beneath the prefix, including the prefix itself (zero
// directories), so patterns like config.d/**/*.conf work; ordinary *, ?
// and [classes] are filepath.Glob per segment.
//
// missing is true when a pattern with no glob characters names a path that
// is not a regular file, or when a glob matches nothing. The returned slice
// is only existing regular files (or symlinks to them), as reached - a
// symlink is not resolved, so a dotfiles checkout still appears as the
// path under ~/.ssh.
func Expand(token, userSSHDir string) (matches []string, missing bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false
	}
	expanded := expandTilde(token)
	full := expanded
	if !filepath.IsAbs(expanded) {
		full = filepath.Join(userSSHDir, expanded)
	}
	full = filepath.Clean(full)
	found, err := expandGlob(full)
	if err != nil {
		return nil, true
	}
	var files []string
	for _, p := range found {
		if isRegular(p) {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, true
	}
	return files, false
}

// ExpandLine splits an Include value on whitespace and expands every token.
// Missing tokens are skipped; the caller records them from Expand.
func ExpandLine(value, userSSHDir string) (files []string, missing []string) {
	for _, token := range strings.Fields(value) {
		found, miss := Expand(token, userSSHDir)
		files = append(files, found...)
		if miss {
			missing = append(missing, token)
		}
	}
	return files, missing
}

func expandTilde(p string) string {
	if p == "~" {
		if home := homeDir(""); home != "" {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home := homeDir(""); home != "" {
			return filepath.Join(home, p[2:])
		}
		return p
	}
	if strings.HasPrefix(p, "~") {
		rest := p[1:]
		name, rem, _ := strings.Cut(rest, "/")
		if name == "" {
			return p
		}
		if home := homeDir(name); home != "" {
			if rem == "" && !strings.Contains(rest, "/") {
				return home
			}
			return filepath.Join(home, rem)
		}
	}
	return p
}

func homeDir(username string) string {
	if username == "" {
		if h := os.Getenv("HOME"); h != "" {
			return h
		}
		u, err := user.Current()
		if err != nil {
			return ""
		}
		return u.HomeDir
	}
	u, err := user.Lookup(username)
	if err != nil {
		return ""
	}
	return u.HomeDir
}

func expandGlob(full string) ([]string, error) {
	if !hasMeta(full) {
		return []string{full}, nil
	}
	vol := filepath.VolumeName(full)
	rest := full[len(vol):]
	sep := string(filepath.Separator)
	// An absolute path starts with the separator after the volume.
	var bases []string
	segs := strings.Split(rest, sep)
	switch {
	case filepath.IsAbs(full):
		bases = []string{vol + sep}
		if len(segs) > 0 && segs[0] == "" {
			segs = segs[1:]
		}
	default:
		bases = []string{"."}
	}
	for _, seg := range segs {
		if seg == "" || seg == "." {
			continue
		}
		var next []string
		for _, base := range bases {
			switch {
			case seg == "**":
				next = append(next, base)
				_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return nil
					}
					if d.IsDir() && path != base {
						next = append(next, path)
					}
					return nil
				})
			case hasMeta(seg):
				found, _ := filepath.Glob(filepath.Join(base, seg))
				next = append(next, found...)
			default:
				p := filepath.Join(base, seg)
				if _, err := os.Lstat(p); err == nil {
					next = append(next, p)
				}
			}
		}
		bases = uniquePaths(next)
		if len(bases) == 0 {
			return nil, nil
		}
	}
	return bases, nil
}

func uniquePaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		c := filepath.Clean(p)
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func isRegular(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
