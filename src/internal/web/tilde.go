package web

import (
	"os"
	"path/filepath"
	"strings"
)

// homeDirs is the user's home directory as the pages may meet it: as $HOME
// says, and with symlinks resolved (on macOS a temporary home under /var is
// really under /private/var). Longest first, so the resolved form wins.
var homeDirs = func() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(home) {
		return nil
	}
	dirs := []string{filepath.Clean(home)}
	if real, err := filepath.EvalSymlinks(home); err == nil && real != dirs[0] {
		dirs = append(dirs, real)
	}
	if len(dirs) == 2 && len(dirs[1]) > len(dirs[0]) {
		dirs[0], dirs[1] = dirs[1], dirs[0]
	}
	return dirs
}

// tildePath shows a path under the home directory as ~/…, the way the CLI
// and the docs write it; any other path is returned unchanged.
func tildePath(p string, homes []string) string {
	for _, h := range homes {
		if p == h {
			return "~"
		}
		if rest, ok := strings.CutPrefix(p, h+string(filepath.Separator)); ok {
			return "~" + string(filepath.Separator) + rest
		}
	}
	return p
}

// tildeText rewrites every home-directory path inside free text (a doctor
// report, an audit line) the same way. Only a whole home directory followed
// by a separator, a space or the end is replaced, so /Users/ann does not
// match inside /Users/anne.
func tildeText(s string, homes []string) string {
	for _, h := range homes {
		var b strings.Builder
		for {
			i := strings.Index(s, h)
			if i < 0 {
				b.WriteString(s)
				break
			}
			end := i + len(h)
			if end == len(s) || s[end] == filepath.Separator || s[end] == ' ' || s[end] == ')' || s[end] == '\n' {
				b.WriteString(s[:i])
				b.WriteString("~")
			} else {
				b.WriteString(s[:end])
			}
			s = s[end:]
		}
		s = b.String()
	}
	return s
}
