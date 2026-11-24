package sshcfg

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// hasMeta reports whether p holds a glob character OpenSSH's Include
// understands (*, ?, or a character class), or a ** segment.
func hasMeta(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

// MaxGlobMatches is how many paths one Include token may expand to.
// A larger glob is truncated, with a warning, so a pattern like /* cannot
// stall the process.
const MaxGlobMatches = 4096

// MaxConfigBytes is the largest ssh config file that is read. A bigger
// file is skipped with a warning (it is not a typical config).
const MaxConfigBytes = 2 << 20

// errGlobLimited is returned by expandGlob when the match cap is reached.
var errGlobLimited = errors.New("include glob truncated")

// Tokens splits an Include (or similar) value the way OpenSSH argv-splits:
// whitespace separates tokens, double or single quotes keep spaces, and a
// backslash escapes the next character outside single quotes.
func Tokens(value string) []string {
	var out []string
	var b strings.Builder
	quote := rune(0)
	escape := false
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range value {
		switch {
		case escape:
			b.WriteRune(r)
			escape = false
		case r == '\\' && quote != '\'':
			escape = true
		case quote != 0:
			if r == quote {
				quote = 0
				break
			}
			b.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
		}
	}
	if escape {
		b.WriteByte('\\')
	}
	flush()
	return out
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
	files, missing, _ := expandToken(token, userSSHDir)
	return files, missing
}

// ExpandLine splits an Include value with Tokens (so quoted paths may
// contain spaces) and expands every token. Missing tokens are listed;
// truncated is set when a glob hit MaxGlobMatches.
func ExpandLine(value, userSSHDir string) (files []string, missing []string, truncated bool) {
	for _, token := range Tokens(value) {
		found, miss, trunc := expandToken(token, userSSHDir)
		files = append(files, found...)
		if miss {
			missing = append(missing, token)
		}
		if trunc {
			truncated = true
		}
	}
	return files, missing, truncated
}

func expandToken(token, userSSHDir string) (matches []string, missing, truncated bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, false, false
	}
	expanded := retargetSSHDir(expandTilde(token), userSSHDir)
	full := expanded
	if !filepath.IsAbs(expanded) {
		full = filepath.Join(userSSHDir, expanded)
	}
	full = filepath.Clean(full)
	found, err := expandGlob(full)
	truncated = errors.Is(err, errGlobLimited)
	if err != nil && !truncated {
		return nil, true, false
	}
	var files []string
	for _, p := range found {
		if isRegular(p) {
			files = append(files, p)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, true, truncated
	}
	return files, false, truncated
}

// retargetSSHDir maps a path under the process home's ~/.ssh to userSSHDir,
// so a sandbox (whose ~/.ssh is not named .ssh, or whose $HOME is still the
// real home) reads its own tree the way the real one would.
func retargetSSHDir(expanded, userSSHDir string) string {
	if expanded == "" || userSSHDir == "" {
		return expanded
	}
	home := homeDir("")
	if home == "" {
		return expanded
	}
	realSSH := filepath.Join(home, ".ssh")
	rel, err := filepath.Rel(realSSH, expanded)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return expanded
	}
	if rel == "." {
		return userSSHDir
	}
	return filepath.Join(userSSHDir, rel)
}

func expandTilde(p string) string {
	if p == "~" {
		if home := homeDir(""); home != "" {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home := homeDir(""); home != "" {
			return filepath.Join(home, filepath.FromSlash(filepath.ToSlash(p[2:])))
		}
		return p
	}
	if strings.HasPrefix(p, "~") {
		rest := p[1:]
		name, rem, _ := strings.Cut(filepath.ToSlash(rest), "/")
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
		if h := os.Getenv("USERPROFILE"); h != "" {
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
						if len(next) > MaxGlobMatches {
							return filepath.SkipAll
						}
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
		if len(bases) > MaxGlobMatches {
			return bases[:MaxGlobMatches], errGlobLimited
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
