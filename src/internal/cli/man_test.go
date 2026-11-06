package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every command's help is laid out as a manual page, in the same sections, in
// the same order.
func TestHelpIsLaidOutAsAManualPage(t *testing.T) {
	editHome(t)
	for _, args := range [][]string{{"--help"}, {"ssh", "--help"}, {"ssh", "deploy", "--help"}, {"auth", "help", "set"}} {
		out := run(t, args...)
		last := -1
		for _, sec := range []string{"NAME", "SYNOPSIS", "DESCRIPTION", "SEE ALSO"} {
			i := strings.Index(out, "\n"+sec+"\n")
			if sec == "NAME" {
				i = strings.Index(out, sec+"\n")
			}
			if i < 0 || i < last {
				t.Errorf("%v: section %s missing or out of order:\n%s", args, sec, out)
			}
			last = i
		}
		if strings.Contains(out, "[flags]") || strings.Contains(out, "Usage:") {
			t.Errorf("%v: cobra's own layout leaked through:\n%s", args, out)
		}
	}
}

// authama man writes a page for every visible command, each a troff page with
// the manual's sections; none for a hidden spelling.
func TestManWritesAPageForEveryCommand(t *testing.T) {
	dir := t.TempDir()
	n, err := writeManPages(newRootCmd(), dir, time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	// 94 pages on 2026-11-04.
	if n < 80 {
		t.Fatalf("wrote %d pages; the walk is not reaching the tree", n)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.1"))
	if len(files) != n {
		t.Fatalf("%d files for %d pages", len(files), n)
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		s := string(b)
		for _, sec := range []string{".TH ", ".SH NAME", ".SH SYNOPSIS", ".SH DESCRIPTION", ".SH SEE ALSO"} {
			if !strings.Contains(s, sec) {
				t.Errorf("%s lacks %s", filepath.Base(f), sec)
			}
		}
	}
	for _, want := range []string{"authama.1", "authama-ssh.1", "authama-ssh-deploy.1", "authama-token-set.1", "authama-events.1"} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("no %s", want)
		}
	}
	// The hidden top-level SSH spellings are not pages of their own.
	if _, err := os.Stat(filepath.Join(dir, "authama-deploy.1")); err == nil {
		t.Error("a hidden spelling got a page")
	}
}

// Every list answers ls and every delete rm, as a shell user types them.
func TestListAndDeleteAnswerTheirUnixNames(t *testing.T) {
	root := newRootCmd()
	pairs := 0
	for _, path := range [][2][]string{
		{{"ssh", "list"}, {"ssh", "ls"}},
		{{"token", "list"}, {"auth", "ls"}},
		{{"ssh", "key", "delete"}, {"ssh", "key", "rm"}},
		{{"ssh", "host", "delete"}, {"ssh", "host", "rm"}},
		{{"token", "delete"}, {"token", "rm"}},
	} {
		a, _, err1 := root.Find(path[0])
		b, _, err2 := root.Find(path[1])
		if err1 != nil || err2 != nil || a != b {
			t.Errorf("%v and %v are not the same command (%v, %v)", path[0], path[1], err1, err2)
		}
		pairs++
	}
	if pairs < 5 {
		t.Fatal("no pairs checked")
	}
}
