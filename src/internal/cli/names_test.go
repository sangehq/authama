package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/brand"

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
//   - a word that names a command in both the ssh and token subtrees, or a
//     top-level command and a token command, means the same kind of thing in
//     both and is listed in sharedVerbs with that meaning; any other overlap
//     is one word with two meanings, and fails;
//   - a deprecated spelling (Deprecated set) is hidden and says what to use;
//   - every other hidden top-level command is the v1 spelling of an ssh verb, with
//     the same subcommands and flags, so the two can never drift apart.

// prefixPairs are v1 siblings where one name starts the other. Prefix
// matching is off, so neither can stand for the other; renaming them would
// break every v1 script, so they are listed here with the reason they differ.
var prefixPairs = map[string]string{
	"key/keygen": "key manages the registry; keygen generates a key file (v1 names)",
}

// sharedVerbs are the words the ssh and token groups (or the top level and
// the token group) both use, each with the one meaning it has everywhere. A
// user who knows `ssh list` can guess `token list`; a word that did something
// else in the other group would be a trap, so a new overlap must be added
// here, with the meaning, or renamed.
var sharedVerbs = map[string]string{
	"list":      "print the items of a kind, one line each (ssh list / key list / vm list / token list)",
	"ls":        "the Unix alias of every list, meaning exactly what list does",
	"rm":        "the Unix alias of every delete, meaning exactly what delete does",
	"show":      "everything about one named item (ssh show / config show / token show)",
	"delete":    "remove one named item (host, profile, key, token), asking first at a terminal",
	"apply":     "add or update items from a declarative source (host specs / the token config)",
	"init":      "create starting state without clobbering what exists (home / known_hosts / token config)",
	"import":    "bring in what another tool or an older layout already holds (~/.ssh / an auth-token-manager config)",
	"audit":     "a read-only hygiene report over everything of a kind (keys / token files)",
	"providers": "the catalog of services the group knows how to talk to (SSH key hosts / token services and registries)",
	"status":    "a read-only summary of current state (git checkouts' remotes / the token setup)",
	"set":       "store values given on the command line (one credential / the naming rules)",
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
	shared := map[string]bool{}
	for n, where := range tokenNames {
		for _, other := range []string{sshNames[n], top[n]} {
			if other == "" {
				continue
			}
			if _, ok := sharedVerbs[n]; ok {
				shared[n] = true
				continue
			}
			t.Errorf("%q is both %s and %s: one word, two meanings (rename one, or add it to sharedVerbs with the meaning both share)", n, where, other)
		}
	}
	for n := range sharedVerbs {
		if !shared[n] {
			t.Errorf("sharedVerbs lists %q, which the token group and the ssh group (or the top level) no longer share; remove it", n)
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
		// tui is the stub left where the removed menu was (v1RemovedVerbs).
		if !c.Hidden || c.Deprecated != "" || c.Name() == "help" || c.Name() == "completion" || c.Name() == "tui" {
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
	var got, aliases []string
	for _, c := range newRootCmd().Commands() {
		if !c.Hidden && c.Name() != "help" && c.Name() != "completion" {
			got = append(got, c.Name())
			aliases = append(aliases, c.Aliases...)
		}
	}
	sort.Strings(got)
	want := append([]string(nil), brand.TopLevel...)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("visible top level %v, brand.TopLevel %v", got, want)
	}
	sort.Strings(aliases)
	wantAliases := append([]string(nil), brand.TopLevelAliases...)
	sort.Strings(wantAliases)
	if strings.Join(aliases, " ") != strings.Join(wantAliases, " ") {
		t.Errorf("visible top-level aliases %v, brand.TopLevelAliases %v", aliases, wantAliases)
	}
	for in, out := range map[string]string{
		"reconcile": "authama ssh reconcile",
		"host add":  "authama ssh host add",
		"init":      "authama init",
		"token set": "authama token set",
		"auth set":  "authama auth set",
		"":          "authama",
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
	}
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if keep[rel] || strings.HasSuffix(rel, "_test.go") ||
			!strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".html") {
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
