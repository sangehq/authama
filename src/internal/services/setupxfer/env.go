package setupxfer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/fs"
)

// EnvText renders the values of names that lookup finds as .env lines, every
// value double-quoted and escaped the way settings.ParseDotenv reads it back,
// and lists the names it found no value for. Values are never logged.
func EnvText(names []string, lookup func(string) (string, bool)) (text string, missing []string) {
	var b strings.Builder
	b.WriteString("# token values exported by authama setup export --encrypt\n")
	for _, n := range names {
		v, ok := lookup(n)
		if !ok || v == "" {
			missing = append(missing, n)
			continue
		}
		b.WriteString(n + "=" + Quote(v) + "\n")
	}
	return b.String(), missing
}

// Quote double-quotes a .env value, escaping what settings.ParseDotenv
// unescapes: backslash, quote, dollar, backtick, and line breaks.
func Quote(v string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`", "\n", `\n`, "\r", `\r`)
	return `"` + r.Replace(v) + `"`
}

// MergeEnv merges the KEY=value lines of text into the .env file at path
// under policy. A name that is new is appended; one whose value differs is
// settled by policy, its line rewritten in place on Replace; every other line,
// comments included, is left as it was. Before a value is replaced the file
// is copied, owner-only, into backupDir. Items name variables, never values.
func MergeEnv(path, text, backupDir string, policy Policy) (Result, error) {
	return mergeEnv(path, text, backupDir, policy, true)
}

// PlanEnv is what MergeEnv would do, writing nothing.
func PlanEnv(path, text string, policy Policy) (Result, error) {
	res, err := mergeEnv(path, text, "", policy, false)
	return res, err
}

func mergeEnv(path, text, backupDir string, policy Policy, write bool) (Result, error) {
	var res Result
	incoming, warns := settings.ParseDotenv(strings.NewReader(text))
	if len(warns) > 0 {
		return res, fmt.Errorf("the export's token values do not parse: %s", strings.Join(warns, "; "))
	}
	cur, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return res, err
	}
	have := map[string]string{}
	pairs, _ := settings.ParseDotenv(strings.NewReader(string(cur)))
	for _, p := range pairs {
		if _, dup := have[p.Key]; !dup {
			have[p.Key] = p.Value
		}
	}
	replace := map[string]string{}
	var add []string
	sort.Slice(incoming, func(i, j int) bool { return incoming[i].Key < incoming[j].Key })
	for _, p := range incoming {
		old, ok := have[p.Key]
		st := StateAdd
		if ok {
			st = settle(old == p.Value, policy)
		}
		res.Items = append(res.Items, Item{Area: "token", Name: p.Key, State: st})
		switch st {
		case StateAdd:
			add = append(add, p.Key+"="+Quote(p.Value))
		case StateReplace:
			replace[p.Key] = p.Value
		}
	}
	if policy == Fail {
		if c := res.Conflicts(); len(c) > 0 {
			return res, ConflictError{Items: c}
		}
	}
	if !write || (len(add) == 0 && len(replace) == 0) {
		return res, nil
	}
	if len(replace) > 0 && len(cur) > 0 {
		if err := fs.WriteTextAtomic(filepath.Join(backupDir, "config", ".env"), string(cur), 0o600); err != nil {
			return res, fmt.Errorf("keeping .env before replacing values in it: %w", err)
		}
	}
	lines := strings.SplitAfter(string(cur), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, line := range lines {
		one, _ := settings.ParseDotenv(strings.NewReader(line))
		if len(one) != 1 {
			continue
		}
		if v, ok := replace[one[0].Key]; ok {
			lines[i] = one[0].Key + "=" + Quote(v) + "\n"
			delete(replace, one[0].Key) // only the first, which is the one LoadDotenv reads
		}
	}
	out := strings.Join(lines, "")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	for _, a := range add {
		out += a + "\n"
	}
	return res, fs.WriteTextAtomic(path, out, 0o600)
}
