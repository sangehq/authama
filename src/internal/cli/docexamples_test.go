package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/brand"
)

// docsRoot is the repository root, relative to this package.
const docsRoot = "../../.."

// docCommandLine matches a line in a fenced code block that runs the tool:
// "authama …" or "sshmgr …", optionally after a "$ " prompt.
var docCommandLine = regexp.MustCompile(`^\s*(?:\$\s+)?(authama|sshmgr)\s+(.*)$`)

// docExampleFiles lists every Markdown file whose code blocks are checked.
// The CHANGELOG is left out: its pre-0.1.0 history quotes commands as they
// were then.
func docExampleFiles(t *testing.T) []string {
	t.Helper()
	files := []string{"README.md", "CONTRIBUTING.md", "SECURITY.md"}
	err := filepath.WalkDir(filepath.Join(docsRoot, "docs"), func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			rel, _ := filepath.Rel(docsRoot, p)
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// Every command line in the docs' code blocks is one this binary accepts:
// the command exists, every flag is one it takes, and the positional
// arguments satisfy its Args rule. Nothing runs. This is the docs-side twin
// of TestEveryCommandHasExamplesThatParse; docs/reference/cli.md is skipped
// because it is generated from those examples.
func TestDocExamplesParse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, rel := range docExampleFiles(t) {
		if rel == filepath.Join("docs", "reference", "cli.md") {
			continue
		}
		f, err := os.Open(filepath.Join(docsRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		inFence, lang, n := false, "", 0
		var pending string
		for sc.Scan() {
			n++
			line := sc.Text()
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				lang = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				continue
			}
			// Only shell blocks: text blocks show output, which may quote commands.
			if inFence && (lang != "" && lang != "sh" && lang != "bash" && lang != "shell" && lang != "console" && lang != "powershell") {
				continue
			}
			if !inFence {
				for _, span := range inlineCode.FindAllStringSubmatch(line, -1) {
					checkDocCommand(t, rel, n, span[1], true)
				}
				continue
			}
			if pending != "" {
				line = pending + " " + strings.TrimSpace(line)
				pending = ""
			}
			if strings.HasSuffix(line, "\\") {
				pending = strings.TrimSuffix(line, "\\")
				continue
			}
			checkDocCommand(t, rel, n, line, false)
		}
		_ = f.Close()
	}
}

// inlineCode matches an inline code span that runs the tool.
var inlineCode = regexp.MustCompile("`((?:authama|sshmgr) [^`]+)`")

// An inline span often names a command in prose ("the web UI's equivalent of
// `authama ssh profile add`") rather than giving a complete command line, so
// for one of those only the command and its flag names are checked.
func checkDocCommand(t *testing.T, rel string, n int, line string, inline bool) {
	t.Helper()
	m := docCommandLine.FindStringSubmatch(line)
	if m == nil {
		return
	}
	// Cut at shell plumbing: what follows is another command.
	rest := m[2]
	for _, sep := range []string{" | ", " && ", " || ", " ; ", " > ", " 2>", " >>", " < "} {
		if i := strings.Index(rest, sep); i >= 0 {
			rest = rest[:i]
		}
	}
	if strings.ContainsAny(rest, "<>[]{}|…$`") || strings.Contains(rest, "...") {
		return // a usage pattern or a shell expansion, not a literal command
	}
	args := splitExample(brand.Command + " " + rest)
	root := newRootCmd()
	// cobra adds `completion` only when the root executes; add it here so the
	// docs can show it.
	root.InitDefaultCompletionCmd()
	cmd, extra, err := root.Find(args)
	where := rel + ":" + strconv.Itoa(n)
	if err != nil {
		t.Errorf("%s: %q: %v", where, strings.TrimSpace(line), err)
		return
	}
	if cmd == root && len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		t.Errorf("%s: %q: no such command", where, strings.TrimSpace(line))
		return
	}
	// The docs teach the grouped form (authama ssh keygen); the bare
	// legacy verbs (sshmgr keygen) are still accepted, and only the
	// migration guide shows them.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") && !isTopLevel(args[0]) && rel != filepath.Join("docs", "migration.md") {
		t.Errorf("%s: %q: use the grouped form, %s", where, strings.TrimSpace(line), brand.Line(strings.Join(args, " ")))
	}
	if err := cmd.ParseFlags(extra); err != nil {
		if inline && strings.Contains(err.Error(), "needs an argument") {
			return
		}
		t.Errorf("%s: %q: %v", where, strings.TrimSpace(line), err)
		return
	}
	if pos := cmd.Flags().Args(); len(cmd.Commands()) > 0 && len(pos) > 0 {
		t.Errorf("%s: %q: %s has no subcommand %q", where, strings.TrimSpace(line), cmd.CommandPath(), pos[0])
		return
	}
	if cmd.Args != nil && !inline {
		if err := cmd.Args(cmd, cmd.Flags().Args()); err != nil {
			t.Errorf("%s: %q: %v", where, strings.TrimSpace(line), err)
		}
	}
}

func isTopLevel(word string) bool {
	if word == "help" || word == "completion" {
		return true
	}
	for _, t := range brand.TopLevel {
		if word == t {
			return true
		}
	}
	return false
}
