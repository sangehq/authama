package settings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/fs"
)

// Problems is `settings validate` with no FILE: .env's syntax and permissions,
// then authama.json against the schema. An absent authama.json is not a
// problem. The CLI and the web UI's Validate page both read it.
func Problems(envFile, settingsFile string) []string {
	return append(DotenvProblems(envFile), FileProblems(settingsFile, false)...)
}

// FileProblems checks one settings file against the schema. named is true
// when the file was given on the command line, where a missing file is an
// error rather than "every setting at its default".
func FileProblems(target string, named bool) []string {
	var problems []string
	if b, err := os.ReadFile(target); err == nil {
		if _, err := Parse(target, b); err != nil {
			var ve *ValidationError
			if errors.As(err, &ve) {
				for _, pr := range ve.Problems {
					problems = append(problems, filepath.Base(target)+": "+pr)
				}
			} else {
				problems = append(problems, err.Error())
			}
		}
	} else if named || !errors.Is(err, os.ErrNotExist) {
		problems = append(problems, err.Error())
	}
	return problems
}

// DotenvProblems checks a .env without loading it.
func DotenvProblems(path string) []string {
	var out []string
	r := LoadDotenv(path, Env{
		Lookup: func(string) (string, bool) { return "", true }, // everything "set": load nothing
		Setenv: func(string, string) error { return nil },
	})
	if r.Refused != "" {
		out = append(out, r.Refused)
	}
	out = append(out, r.Warnings...)
	if f, err := os.Open(path); err == nil {
		pairs, _ := ParseDotenv(f)
		_ = f.Close()
		out = append(out, nameProblems(pairs)...)
	}
	return out
}

// legacyPrefixes are the names before AUTHAMA_: sshmgr and ssh-manager wrote
// them, and nothing has read them since they were retired (#146).
var legacyPrefixes = []string{"SSHMGR_", "SSH_MANAGER_"}

// modern is the AUTHAMA_ name an old one stood for, when it is one authama
// reads.
func modern(name string) (string, bool) {
	for _, pre := range legacyPrefixes {
		if rest, ok := strings.CutPrefix(name, pre); ok {
			to := "AUTHAMA_" + rest
			return to, slices.Contains(envnames.Names(), to)
		}
	}
	return "", false
}

// nameProblems reports each .env entry nothing reads, and each value its
// setting cannot take: validate said "ok" over all of them, and the setting
// silently kept its default (review R-GAP-03).
func nameProblems(pairs []Pair) []string {
	known := envnames.Names()
	spec := map[string]Spec{}
	for _, s := range Specs {
		if s.Env != "" {
			spec[s.Env] = s
		}
	}
	var out []string
	for _, p := range pairs {
		at := fmt.Sprintf(".env line %d: ", p.Line)
		switch {
		case isLegacy(p.Key):
			if to, ok := modern(p.Key); ok {
				out = append(out, at+p.Key+" is an old name authama no longer reads; set "+to+" instead")
			} else {
				out = append(out, at+p.Key+" is an old name authama no longer reads")
			}
		case strings.HasPrefix(p.Key, "AUTHAMA_") && !slices.Contains(known, p.Key):
			out = append(out, at+p.Key+" is not a setting authama reads (`authama settings show --effective` lists them)")
		default:
			if sp, ok := spec[p.Key]; ok {
				if err := envValueProblem(sp, p.Value); err != nil {
					out = append(out, at+p.Key+"="+p.Value+": "+err.Error())
				}
			}
		}
	}
	return out
}

func isLegacy(name string) bool {
	for _, pre := range legacyPrefixes {
		if strings.HasPrefix(name, pre) {
			return true
		}
	}
	return false
}

// envValueProblem checks a value as the environment variable is read. A
// boolean is read as true for 1, true, yes or on and false for anything
// else, so any other word is a typo that quietly means false.
func envValueProblem(sp Spec, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil // empty means unset: the default applies
	}
	if sp.Kind == KindBool {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "on", "0", "false", "no", "off":
			return nil
		}
		return fmt.Errorf("is read as false: use true or false")
	}
	_, err := sp.literal(value)
	return err
}

// RenameLegacyNames rewrites, in place, each old SSHMGR_ or SSH_MANAGER_ name
// in a .env to the AUTHAMA_ name it stood for, when authama reads that name
// and the file does not already set it. Everything else, comments included,
// is kept as it was. It returns the names renamed. Migrating a home calls it,
// so the copy's settings still apply.
func RenameLegacyNames(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pairs, _ := ParseDotenv(bytes.NewReader(b))
	set := map[string]bool{}
	for _, p := range pairs {
		set[p.Key] = true
	}
	rename := map[int]string{} // line -> new name
	var renamed []string
	for _, p := range pairs {
		if to, ok := modern(p.Key); ok && !set[to] {
			rename[p.Line] = to
			set[to] = true
			renamed = append(renamed, p.Key+" -> "+to)
		}
	}
	if len(rename) == 0 {
		return nil, nil
	}
	lines := strings.Split(string(b), "\n")
	for n, to := range rename {
		i := n - 1
		if i < 0 || i >= len(lines) {
			continue
		}
		if eq := strings.Index(lines[i], "="); eq >= 0 {
			body := strings.TrimLeft(lines[i], " \t")
			lead := lines[i][:len(lines[i])-len(body)]
			prefix := ""
			if strings.HasPrefix(body, "export ") {
				prefix = "export "
			}
			lines[i] = lead + prefix + to + lines[i][eq:]
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return renamed, fs.WriteTextAtomic(path, strings.Join(lines, "\n"), fi.Mode().Perm())
}
