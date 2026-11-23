//go:build e2e && !windows

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The docs' commands, run: every read-only command line in README.md and
// docs/ (code blocks and inline spans) runs against a seeded throwaway home,
// and none may be refused as a usage error - an unknown command or flag, or
// the wrong number of arguments. Whether it then finds what it names is the
// home's business; that the docs spell a command this build runs is theirs.
// TestDocExamplesParse checks the same lines against the command tree without
// running them; this catches what only a run shows (argument validators,
// flag combinations a command refuses, a panic).
func TestQADocCommandsRun(t *testing.T) {
	b := newQABox(t)
	port := closedPort(t)
	for _, s := range []qaStep{
		ok("init", "--yes"),
		ok("ssh", "profile", "add", "work"),
		ok("ssh", "host", "add", "work", "web1", "--hostname", "127.0.0.1", "--port", port, "--user", "deploy"),
		ok("ssh", "reconcile", "--yes"),
	} {
		b.step(s)
	}
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, line := range docCommandLines(t, repo) {
		args, ok := docArgs(line.text)
		if !ok {
			continue
		}
		r := b.run("", args...)
		ran++
		both := r.out + r.err
		for _, refused := range []string{"unknown command", "unknown flag", "unknown shorthand flag",
			"accepts at most", "accepts between", "requires at least", "accepts 0 arg", "accepts 1 arg", "accepts 2 arg",
			"if any flags in the group", "none of the others can be", "flag needs an argument"} {
			if strings.Contains(r.err, refused) {
				t.Errorf("%s:%d: `%s` is refused: %s", line.file, line.n, line.text, strings.TrimSpace(both))
				break
			}
		}
	}
	if ran < 100 {
		t.Errorf("ran only %d doc commands", ran)
	}
	t.Logf("ran %d read-only doc commands", ran)
}

type docLine struct {
	file string
	n    int
	text string
}

var docInline = regexp.MustCompile("`(authama [^`]+)`")

func docCommandLines(t *testing.T, repo string) []docLine {
	t.Helper()
	files := []string{filepath.Join(repo, "README.md")}
	_ = filepath.WalkDir(filepath.Join(repo, "docs"), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") &&
			// The generated reference repeats every command's own examples.
			filepath.Base(path) != "cli.md" {
			files = append(files, path)
		}
		return nil
	})
	var out []docLine
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(repo, f)
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		inBlock := false
		for n := 1; sc.Scan(); n++ {
			text := sc.Text()
			if strings.HasPrefix(strings.TrimSpace(text), "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				trimmed := strings.TrimPrefix(strings.TrimSpace(text), "$ ")
				if strings.HasPrefix(trimmed, "authama ") {
					if i := strings.Index(trimmed, " #"); i > 0 {
						trimmed = trimmed[:i]
					}
					out = append(out, docLine{rel, n, strings.TrimSpace(trimmed)})
				}
				continue
			}
			for _, m := range docInline.FindAllStringSubmatch(text, -1) {
				out = append(out, docLine{rel, n, m[1]})
			}
		}
		_ = fh.Close()
	}
	return out
}

// readOnly are the last words of commands that only read; anything else in
// the docs is not run here (the walk covers the writes in a home it controls).
var readOnly = map[string]bool{
	"list": true, "show": true, "status": true, "check": true, "validate": true, "doctor": true,
	"diff": true, "expiry": true, "audit": true, "naming": true, "view": true, "providers": true,
	"history": true, "events": true, "version": true, "path": true, "examples": true, "checklist": true,
	"companion": true, "resolve": true,
}

// takesAName are the read-only commands whose last word is the thing they
// read, not a subcommand.
var takesAName = map[string]bool{"show": true, "view": true, "diff": true, "expiry": true, "resolve": true, "status": true}

// docArgs splits a doc command line into arguments when it is safe to run:
// read-only, with no placeholder, shell syntax or environment, and nothing
// that reaches outside the sandbox.
func docArgs(line string) ([]string, bool) {
	if strings.ContainsAny(line, "<>|$;&`*[]{}…") || strings.Contains(line, "...") || strings.Contains(line, "\\") {
		return nil, false
	}
	words, ok := shellWords(line)
	if !ok || len(words) < 2 || words[0] != "authama" {
		return nil, false
	}
	args := words[1:]
	var verbs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			break
		}
		verbs = append(verbs, a)
	}
	if len(verbs) == 0 {
		return nil, false
	}
	for _, never := range []string{"serve", "load", "copy", "install", "uninstall", "send", "ping", "test", "net", "deliver", "mcp", "termius"} {
		if slices.Contains(verbs, never) {
			return nil, false
		}
	}
	last := verbs[len(verbs)-1]
	readOnlyRun := slices.Contains(args, "--dry-run") || slices.Contains(args, "-n") || slices.Contains(args, "--print") ||
		slices.Contains(args, "--check")
	switch {
	case readOnly[last]:
	case len(verbs) >= 2 && takesAName[verbs[len(verbs)-2]]: // `ssh show web1`, `ssh view web1`
	case readOnlyRun:
	default:
		return nil, false
	}
	return args, true
}

// shellWords splits on spaces, honouring double and single quotes.
func shellWords(s string) ([]string, bool) {
	var out []string
	var cur strings.Builder
	var quote rune
	in := false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '"' || r == '\'':
			quote, in = r, true
		case r == ' ' || r == '\t':
			if in || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 {
		return nil, false
	}
	if in || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, true
}
