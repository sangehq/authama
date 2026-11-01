package cli

import (
	"github.com/simtabi/ssh-manager/src/v3/internal/brand"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The command tree carries two features under one binary. A name that means
// one thing under `ssh` and another under `token`, two siblings one typo apart,
// or a prefix that could stand for either is how a user ends up running the
// wrong verb, so the tree's names are held to these rules:
//
//   - cobra's prefix matching stays off: `authama ssh ro` is an error, never a
//     guess between rotate and rollback;
//   - siblings' names and aliases are unique and at least two edits apart;
//   - no word names a command in both the ssh and token subtrees, or names a
//     top-level command and a token command;
//   - a deprecated spelling (Deprecated set) is hidden and says what to use;
//   - every other hidden top-level command is the v1 spelling of an ssh verb, with
//     the same subcommands and flags, so the two can never drift apart.

// prefixPairs are v1 siblings where one name starts the other. Prefix
// matching is off, so neither can stand for the other; renaming them would
// break every v1 script, so they are listed here with the reason they differ.
var prefixPairs = map[string]string{
	"key/keygen": "key manages the registry; keygen generates a key file (v1 names)",
}

func names(c *cobra.Command) []string { return append([]string{c.Name()}, c.Aliases...) }

func visible(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, s := range c.Commands() {
		if s.Hidden || s.Name() == "help" || s.Name() == "completion" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func childByName(c *cobra.Command, name string) *cobra.Command {
	for _, s := range c.Commands() {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// subtreeNames is every command name and alias at or below c (c excluded).
func subtreeNames(c *cobra.Command, into map[string]string) {
	for _, s := range visible(c) {
		for _, n := range names(s) {
			into[n] = s.CommandPath()
		}
		subtreeNames(s, into)
	}
}

func TestTheCommandNamesAreDistinct(t *testing.T) {
	if cobra.EnablePrefixMatching {
		t.Fatal("cobra.EnablePrefixMatching is on: an abbreviation could resolve to either of two verbs")
	}
	root := newRootCmd()
	used := map[string]bool{}

	var siblings func(c *cobra.Command)
	siblings = func(c *cobra.Command) {
		kids := visible(c)
		var all []string
		for _, k := range kids {
			all = append(all, names(k)...)
			siblings(k)
		}
		seen := map[string]bool{}
		for _, n := range all {
			if seen[n] {
				t.Errorf("%s: %q names two commands", c.CommandPath(), n)
			}
			seen[n] = true
		}
		for i, a := range all {
			for _, b := range all[i+1:] {
				if d := levenshtein(a, b); d < 2 {
					t.Errorf("%s: %q and %q are %d edit apart; one typo runs the other", c.CommandPath(), a, b, d)
				}
				lo, hi := a, b
				if len(hi) < len(lo) {
					lo, hi = hi, lo
				}
				if strings.HasPrefix(hi, lo) {
					used[lo+"/"+hi] = true
					if _, ok := prefixPairs[lo+"/"+hi]; !ok {
						t.Errorf("%s: %q starts %q; pick a name that does not read as its abbreviation", c.CommandPath(), lo, hi)
					}
				}
			}
		}
	}
	siblings(root)
	for pair := range prefixPairs {
		if !used[pair] {
			t.Errorf("prefixPairs lists %q, which no longer exists; remove it", pair)
		}
	}

	ssh, token := childByName(root, sshGroup), childByName(root, "token")
	if ssh == nil || token == nil {
		t.Fatal("the root has no ssh or token group")
	}
	sshNames, tokenNames := map[string]string{}, map[string]string{}
	subtreeNames(ssh, sshNames)
	subtreeNames(token, tokenNames)
	top := map[string]string{}
	for _, c := range root.Commands() {
		for _, n := range names(c) {
			top[n] = c.CommandPath()
		}
	}
	for n, where := range tokenNames {
		if other, ok := sshNames[n]; ok {
			t.Errorf("%q is both %s and %s: one word, two meanings", n, where, other)
		}
		if other, ok := top[n]; ok {
			t.Errorf("%q is both %s and %s", n, where, other)
		}
	}
	if len(tokenNames) == 0 || len(sshNames) == 0 {
		t.Fatal("walked an empty group")
	}
}

func flagNames(c *cobra.Command) []string {
	var out []string
	c.LocalFlags().VisitAll(func(f *pflag.Flag) { out = append(out, f.Name+"/"+f.Shorthand) })
	sort.Strings(out)
	return out
}

func shape(c *cobra.Command, prefix string, into map[string]string) {
	into[prefix] = strings.Join(flagNames(c), ",") + "|" + strings.Join(c.Aliases, ",")
	for _, s := range c.Commands() {
		shape(s, prefix+" "+s.Name(), into)
	}
}

func TestEveryHiddenTopLevelCommandIsAnSSHVerb(t *testing.T) {
	root := newRootCmd()
	ssh := childByName(root, sshGroup)
	hidden := 0
	for _, c := range root.Commands() {
		if !c.Hidden || c.Deprecated != "" || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		hidden++
		twin := childByName(ssh, c.Name())
		if twin == nil {
			t.Errorf("hidden %q has no `ssh %s`", c.Name(), c.Name())
			continue
		}
		if twin.Hidden {
			t.Errorf("`ssh %s` is hidden too; the grouped spelling is the documented one", c.Name())
		}
		a, b := map[string]string{}, map[string]string{}
		shape(c, c.Name(), a)
		shape(twin, c.Name(), b)
		for k, v := range a {
			if b[k] != v {
				t.Errorf("%s: the hidden spelling has %q, `ssh` has %q", k, v, b[k])
			}
		}
		if len(a) != len(b) {
			t.Errorf("%s: %d commands in the hidden spelling, %d under ssh", c.Name(), len(a), len(b))
		}
	}
	if hidden != len(visible(ssh)) {
		t.Errorf("%d hidden top-level spellings for %d ssh verbs", hidden, len(visible(ssh)))
	}
}

// brand.Line decides whether a recorded verb prints as `authama <verb>` or
// `authama ssh <verb>`; its list has to be the tree's top level.
func TestBrandTopLevelIsTheVisibleRoot(t *testing.T) {
	var got []string
	for _, c := range newRootCmd().Commands() {
		if !c.Hidden && c.Name() != "help" && c.Name() != "completion" {
			got = append(got, c.Name())
		}
	}
	sort.Strings(got)
	want := append([]string(nil), brand.TopLevel...)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visible top level %v, brand.TopLevel %v", got, want)
	}
	for in, out := range map[string]string{
		"reconcile":   "authama ssh reconcile",
		"host add":    "authama ssh host add",
		"init":        "authama init",
		"token store": "authama token store",
		"":            "authama",
	} {
		if g := brand.Line(in); g != out {
			t.Errorf("brand.Line(%q) = %q, want %q", in, g, out)
		}
	}
}

// Messages tell people what to run next. After the rename they must say
// `authama ssh <verb>`: the old spelling still works, but a hint is the one
// place new users learn the command, so it must teach the current one. The
// files listed write the spelling into config and key names on purpose.
func TestNoMessageTeachesTheOldSpelling(t *testing.T) {
	var verbs []string
	for _, c := range newRootCmd().Commands() {
		verbs = append(verbs, c.Name())
	}
	rx := regexp.MustCompile(`\bsshmgr (` + strings.Join(verbs, "|") + `)\b`)
	keep := map[string]bool{
		"core/renderer/renderer.go": true, // the managed-block markers already on disk
		"core/renderer/gitvm.go":    true, // ProxyCommand for the compat binary
	}
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if keep[rel] || strings.HasSuffix(rel, "_test.go") ||
			!(strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".html")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if m := rx.FindString(line); m != "" {
				t.Errorf("%s:%d says %q; use `%s %s`", rel, i+1, m, brand.Command,
					strings.TrimPrefix(brand.Line(strings.TrimPrefix(m, "sshmgr ")), brand.Command+" "))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
