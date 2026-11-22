package structure

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// identityLine matches an IdentityFile (or CertificateFile) line: its
// leading part, the path (quoted or not) and what trails it.
var identityLine = regexp.MustCompile(`(?i)^(\s*(?:IdentityFile|CertificateFile)(?:\s*=\s*|\s+)"?)([^"\s]+)("?\s*(?:#.*)?)$`)

// handWritten is the ssh config files a person edits: ~/.ssh/config, the
// files in ~/.ssh/config.d and any ~/.ssh/*.conf. Files authama renders under
// the key root are not among them; a re-render rewrites those.
func handWritten(p paths.Paths) []string {
	out := []string{filepath.Join(p.SSHDir, "config")}
	more, _ := filepath.Glob(filepath.Join(p.SSHDir, "config.d", "*"))
	conf, _ := filepath.Glob(filepath.Join(p.SSHDir, "*.conf"))
	out = append(out, more...)
	out = append(out, conf...)
	var files []string
	for _, f := range out {
		if fi, err := os.Lstat(f); err == nil && fi.Mode().IsRegular() {
			files = append(files, f)
		}
	}
	return files
}

// expand resolves an IdentityFile value the way ssh would for the forms that
// name a file in ~/.ssh: ~/, $HOME/, ${HOME}/, %d/ and absolute paths. A
// home-relative .ssh/ resolves to p's ~/.ssh, so a sandbox (whose ~/.ssh is
// not named .ssh) reads its own config the way the real one would.
func expand(p paths.Paths, v string) string {
	for _, pre := range []string{"~/", "$HOME/", "${HOME}/", "%d/"} {
		rest, ok := strings.CutPrefix(v, pre)
		if !ok {
			continue
		}
		if in, ok := strings.CutPrefix(rest, ".ssh/"); ok {
			return filepath.Join(p.SSHDir, filepath.FromSlash(in))
		}
		return filepath.Join(filepath.Dir(p.SSHDir), filepath.FromSlash(rest))
	}
	if filepath.IsAbs(v) {
		return filepath.Clean(v)
	}
	return ""
}

// ConfigRefs maps each private key path a hand-written config names (its
// .pub too, mapped to the private half) to the files naming it, as "~/.ssh/<rel>".
func ConfigRefs(p paths.Paths) map[string][]string {
	out := map[string][]string{}
	for _, f := range handWritten(p) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		seen := map[string]bool{}
		for _, ln := range strings.Split(string(b), "\n") {
			m := identityLine.FindStringSubmatch(strings.TrimRight(ln, "\r"))
			if m == nil {
				continue
			}
			abs := strings.TrimSuffix(expand(p, m[2]), ".pub")
			if abs == "" || seen[abs] {
				continue
			}
			seen[abs] = true
			out[abs] = append(out[abs], "~/.ssh/"+relSSH(p, f))
		}
	}
	for k := range out {
		sort.Strings(out[k])
	}
	return out
}

// rewriteRefs points every IdentityFile/CertificateFile line in the
// hand-written configs that names oldAbs (or its .pub, or its -cert.pub) at
// newAbs instead, written as ~/.ssh/<rel>. It returns the files it changed
// (as "~/.ssh/<rel>"); each keeps its mode.
func rewriteRefs(p paths.Paths, oldAbs, newAbs string) ([]string, error) {
	return rewrite(p, func(abs string) (string, bool) {
		for _, s := range []string{"", "-cert.pub", ".pub"} {
			if abs == oldAbs+s {
				return newAbs + s, true
			}
		}
		return "", false
	})
}

// rewriteDirRefs repoints every line naming a file under oldDir to the same
// file under newDir: a profile directory that moved with a category change.
func rewriteDirRefs(p paths.Paths, oldDir, newDir string) ([]string, error) {
	return rewrite(p, func(abs string) (string, bool) {
		if rest, ok := strings.CutPrefix(abs, oldDir+string(filepath.Separator)); ok {
			return filepath.Join(newDir, rest), true
		}
		return "", false
	})
}

func rewrite(p paths.Paths, to func(abs string) (string, bool)) ([]string, error) {
	var changed []string
	for _, f := range handWritten(p) {
		fi, err := os.Stat(f)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			return changed, err
		}
		lines := strings.Split(string(b), "\n")
		touched := false
		for i, ln := range lines {
			cr := strings.HasSuffix(ln, "\r")
			m := identityLine.FindStringSubmatch(strings.TrimRight(ln, "\r"))
			if m == nil {
				continue
			}
			abs := expand(p, m[2])
			if abs == "" {
				continue
			}
			next, ok := to(abs)
			if !ok {
				continue
			}
			lines[i] = m[1] + "~/.ssh/" + relSSH(p, next) + m[3]
			if cr {
				lines[i] += "\r"
			}
			touched = true
		}
		if !touched {
			continue
		}
		if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")), fi.Mode().Perm()); err != nil {
			return changed, err
		}
		changed = append(changed, "~/.ssh/"+relSSH(p, f))
	}
	return changed, nil
}
