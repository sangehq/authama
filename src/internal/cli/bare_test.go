package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/services/mutation"
)

// whileBareServes, when set, runs while a bare run is serving.
var whileBareServes func()

// runBareRoot runs the root with no command until serve says it is listening,
// then stops it. It returns what was printed.
func runBareRoot(t *testing.T, interactive bool, args ...string) string {
	t.Helper()
	prev := bareIsInteractive
	bareIsInteractive = func() bool { return interactive }
	t.Cleanup(func() { bareIsInteractive = prev })
	_, stop := fakeServe(t)

	root := newRootCmd()
	var out syncBuffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	done := make(chan error, 1)
	go func() { done <- root.Execute() }()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			return out.String()
		case <-deadline:
			stop()
			t.Fatalf("the bare run neither served nor exited:\n%s", out.String())
		case <-time.After(10 * time.Millisecond):
			if strings.Contains(out.String(), "press Ctrl-C to stop") {
				if whileBareServes != nil {
					whileBareServes()
				}
				stop()
				if err := <-done; err != nil {
					t.Fatalf("%v\n%s", err, out.String())
				}
				return out.String()
			}
		}
	}
}

// Bare `authama` on a terminal is `authama serve`: the web UI, on the default
// port, previewing changes.
func TestABareRunOnATerminalOpensTheWebUI(t *testing.T) {
	editHome(t)
	out := runBareRoot(t, true)
	if m := printedURL.FindStringSubmatch(out); m == nil || m[1] != "7822" {
		t.Fatalf("a bare run did not serve on the default port:\n%s", out)
	}
	if !strings.Contains(out, "changes are previewed before they are made") {
		t.Errorf("a bare run should serve changes, not read-only:\n%s", out)
	}
}

// The global flags still apply to the bare run.
func TestABareRunHonoursDevRoot(t *testing.T) {
	editHome(t)
	dir := t.TempDir()
	out := runBareRoot(t, true, "--dev-root", dir)
	if !strings.Contains(out, "dev mode - writing under "+dir) || printedURL.FindString(out) == "" {
		t.Fatalf("--dev-root was not applied to the bare run:\n%s", out)
	}
}

// With no terminal a bare run prints help: serve runs until Ctrl-C, so a
// script that ran bare `authama` would hang.
func TestABareRunWithoutATerminalPrintsHelp(t *testing.T) {
	editHome(t)
	out := runBareRoot(t, false)
	if printedURL.FindString(out) != "" || !strings.Contains(out, "COMMANDS") {
		t.Fatalf("a bare run with no terminal should print help and serve nothing:\n%s", out)
	}
}

// `auth` is the same group as `token`, verb for verb.
func TestAuthIsTheTokenGroup(t *testing.T) {
	root := newRootCmd()
	auth, _, err := root.Find([]string{"auth", "list"})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := root.Find([]string{"token", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if auth != token {
		t.Fatalf("auth list resolves to %q, token list to %q", auth.CommandPath(), token.CommandPath())
	}
}

// Every group answers `help [verb]`, as the root does, and an unknown verb
// is reported against the group with a suggestion.
func TestEveryGroupAnswersHelp(t *testing.T) {
	editHome(t)
	groups := 0
	var walk func(path []string)
	root := newRootCmd()
	walk = func(path []string) {
		c, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("%v: %v", path, err)
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "help" || sub.Hidden || !sub.IsAvailableCommand() {
				continue
			}
			if sub.HasSubCommands() {
				walk(append(append([]string{}, path...), sub.Name()))
			}
		}
		if len(path) == 0 || !c.HasSubCommands() {
			return
		}
		groups++
		out := run(t, append(append([]string{}, path...), "help")...)
		if !strings.Contains(out, "SYNOPSIS") || !strings.Contains(out, c.CommandPath()) {
			t.Errorf("`%s help` printed no help for the group:\n%s", c.CommandPath(), out)
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "help" || sub.Hidden {
				continue
			}
			out := run(t, append(append([]string{}, path...), "help", sub.Name())...)
			if !strings.Contains(out, sub.CommandPath()) {
				t.Errorf("`%s help %s` did not print %s's help:\n%s", c.CommandPath(), sub.Name(), sub.CommandPath(), out)
			}
			break
		}
	}
	walk(nil)
	// ssh, token, settings and the nested groups: measured 2026-11-04.
	if groups < 8 {
		t.Fatalf("only %d groups inspected; the walk is broken", groups)
	}

	root = newRootCmd()
	root.SetArgs([]string{"ssh", "help", "deplyo"})
	var out syncBuffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown command "deplyo" for "ssh"`) || !strings.Contains(err.Error(), "deploy") {
		t.Errorf("an unknown verb after help should be reported with a suggestion, got %v", err)
	}
}

// A bare run is the web UI, which records each change a browser confirms as
// its own operation with the web as its source. No scope may sit around the
// server: it was once a "tui" scope, which labelled the web UI as the menu and
// would claim any event the server saw outside a change.
func TestABareRunServesWithNoHistoryScopeOpen(t *testing.T) {
	editHome(t)
	depth := -1
	whileBareServes = func() { depth = mutation.Depth() }
	t.Cleanup(func() { whileBareServes = nil })
	runBareRoot(t, true)
	if depth != 0 {
		t.Errorf("%d history scope(s) open while a bare run serves, want 0", depth)
	}
}
