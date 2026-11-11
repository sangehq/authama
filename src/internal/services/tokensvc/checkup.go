package tokensvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/util/perms"
)

// Finding is one line of the token audit.
type Finding struct {
	Level  string // ok | absent | warn | note
	Target string
	Text   string
	// Path, Mode and Missing let a page lay the finding out rather than print
	// Text: the file looked at, its permission bits ("0600", or "ACL" on
	// Windows), and for a .gitignore the secret file names it does not cover.
	Path    string
	Mode    string
	Missing []string
}

// Checkup reports, for each target (the set's, or all five), whether its
// file exists and is owner-only, that a present auth.json overrides
// $COMPOSER_AUTH, and whether .gitignore covers the files that hold secrets.
// failed is set when anything needs fixing.
func (e Env) Checkup(set *tokens.Set, project string) (out []Finding, failed bool) {
	targets := set.Targets()
	if len(targets) == 0 {
		targets = tokens.Targets()
	}
	for _, t := range targets {
		path := e.TargetPath(t, project)
		if !exists(path) {
			out = append(out, Finding{Level: "absent", Target: t, Text: fmt.Sprintf("%s: %s  (absent)", t, path), Path: path})
			continue
		}
		ok := ownerOnly(path)
		lvl := "ok"
		if !ok {
			lvl = "warn"
		}
		out = append(out, Finding{Level: lvl, Target: t, Text: fmt.Sprintf("%s: %s  perms=%s", t, path, permString(path)), Path: path, Mode: modeString(path)})
		if !ok {
			out = append(out, Finding{Level: "warn", Target: t, Text: "world/group-readable — `doctor --fix` (or chmod 600) makes it owner-only.", Path: path, Mode: modeString(path)})
			failed = true
		}
		if t == tokens.Composer {
			out = append(out, Finding{Level: "note", Target: t, Text: "a present auth.json OVERRIDES $COMPOSER_AUTH.", Path: path})
		}
	}
	base := e.abs(project)
	if base == "" {
		base = e.Cwd
	}
	gi := filepath.Join(base, ".gitignore")
	if b, err := os.ReadFile(gi); err == nil {
		var rules []string
		for _, ln := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			ln = strings.TrimSpace(ln)
			if ln != "" && !strings.HasPrefix(ln, "#") {
				rules = append(rules, ln)
			}
		}
		var missing []string
		for _, s := range []string{"auth.json", ".npmrc", ".git-credentials", ".netrc"} {
			covered := false
			for _, r := range rules {
				if strings.Contains(r, s) {
					covered = true
				}
			}
			if !covered {
				missing = append(missing, s)
			}
		}
		if len(missing) > 0 {
			out = append(out, Finding{Level: "warn", Text: fmt.Sprintf("%s is missing: %s", gi, strings.Join(missing, ", ")), Path: gi, Missing: missing})
			failed = true
		}
	}
	return out, failed
}

// ownerOnly is "no group or other access" on POSIX; on Windows the ACL
// check stands in.
func ownerOnly(path string) bool { return ownerOnlyOn(platform.IsWindows(), path) }

// ownerOnlyOn is ownerOnly with the platform passed in, so the Windows branch
// is tested on every host (as test_windows.py did by toggling IS_WINDOWS).
func ownerOnlyOn(windows bool, path string) bool {
	if windows {
		return perms.WindowsPermsOK(path)
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().Perm()&0o077 == 0
}

func permString(path string) string { return permStringOn(platform.IsWindows(), path) }

// modeString is a file's permission bits as people write them, "0600", or
// "ACL" on Windows, for a page to show.
func modeString(path string) string {
	fi, err := os.Stat(path)
	switch {
	case err != nil:
		return ""
	case platform.IsWindows():
		return "ACL"
	}
	return fmt.Sprintf("%04o", fi.Mode().Perm())
}

func permStringOn(windows bool, path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	if windows {
		return "ACL"
	}
	return fmt.Sprintf("0o%o", fi.Mode().Perm())
}

// HomeFiles is the doctor's view of the token files: each per-user target
// file that exists, and whether it is owner-only. With fix, a file that is
// not is chmod'ed to SecretMode first (inside the sandbox in dev mode), and
// fixed names it. The project-scoped .gitignore check stays in Checkup: the
// doctor runs from any directory and has no project to judge.
func (e Env) HomeFiles(fix bool) (out []Finding, failed bool, fixed []string) {
	for _, t := range tokens.Targets() {
		path := e.TargetPath(t, "")
		if !exists(path) {
			continue
		}
		if fix && !ownerOnly(path) && e.Guard(path) == nil {
			if err := perms.SetPerms(path, SecretMode); err == nil {
				fixed = append(fixed, path)
			}
		}
		if ownerOnly(path) {
			out = append(out, Finding{Level: "ok", Target: t, Text: fmt.Sprintf("%s: %s  perms=%s", t, path, permString(path)), Path: path, Mode: modeString(path)})
			continue
		}
		out = append(out, Finding{Level: "warn", Target: t, Text: fmt.Sprintf("%s: %s  perms=%s — group/world-readable (doctor --fix, or chmod 600)",
			t, path, permString(path)), Path: path, Mode: modeString(path)})
		failed = true
	}
	return out, failed, fixed
}
