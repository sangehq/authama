package tokensvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/platform"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/perms"
)

// Finding is one line of the token checkup.
type Finding struct {
	Level  string // ok | absent | warn | note
	Target string
	Text   string
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
			out = append(out, Finding{"absent", t, fmt.Sprintf("%s: %s  (absent)", t, path)})
			continue
		}
		ok := ownerOnly(path)
		lvl := "ok"
		if !ok {
			lvl = "warn"
		}
		out = append(out, Finding{lvl, t, fmt.Sprintf("%s: %s  perms=%s", t, path, permString(path))})
		if !ok {
			out = append(out, Finding{"warn", t, "world/group-readable — re-render to fix."})
			failed = true
		}
		if t == tokens.Composer {
			out = append(out, Finding{"note", t, "a present auth.json OVERRIDES $COMPOSER_AUTH."})
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
			out = append(out, Finding{"warn", "", fmt.Sprintf("%s is missing: %s", gi, strings.Join(missing, ", "))})
			failed = true
		}
	}
	return out, failed
}

// ownerOnly is "no group or other access" on POSIX; on Windows the ACL
// check stands in.
func ownerOnly(path string) bool {
	if platform.IsWindows() {
		return perms.PermsOK(path, SecretMode)
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().Perm()&0o077 == 0
}

func permString(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "absent"
	}
	if platform.IsWindows() {
		return "ACL"
	}
	return fmt.Sprintf("0o%o", fi.Mode().Perm())
}
