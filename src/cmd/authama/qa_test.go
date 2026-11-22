//go:build e2e && !windows

// The QA harness: every command's help, every command's flag parsing, and a
// scripted walk of the commands and their main flags on a real binary in a
// throwaway home. Run it with `make qa` (or `make e2e`, which includes it).
//
// The sandbox is sealed: HOME, the config and state homes and the owner key
// store are temp dirs; every AUTHAMA_*, XDG_* and SSH_AUTH_SOCK variable from
// the caller is dropped; hosts are 127.0.0.1 on a closed port, so nothing
// leaves the machine. Commands that would reach outside the sandbox (desktop
// notifications, launchd/systemd installs, the clipboard, the agent, a server
// that keeps running) are checked through their help only.
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

type qaBox struct {
	t    *testing.T
	bin  string
	home string
	env  []string
	dir  string // the working directory commands run in
}

// newQABox builds the binary and seals a home around it.
func newQABox(t *testing.T) *qaBox {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a binary")
	}
	for _, bin := range []string{"go", "ssh-keygen", "ssh"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is required", bin)
		}
	}
	bin := filepath.Join(t.TempDir(), "authama")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "AUTHAMA_") || strings.HasPrefix(k, "XDG_") || strings.HasPrefix(k, "GIT_") ||
			k == "SSH_AUTH_SOCK" || k == "HOME" || k == "USERPROFILE" || k == "GH_TOKEN" || k == "GITHUB_TOKEN" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"HOME="+home,
		"AUTHAMA_CONFIG_DIR="+filepath.Join(home, "cfg"),
		"AUTHAMA_STATE_DIR="+filepath.Join(home, "state"),
		"AUTHAMA_OWNER_STORE=file",
		"AUTHAMA_AUTO_PIN=0",
		"AUTHAMA_NOTIFICATIONS=off",
		// Only a hypervisor no machine here has: the real ones' CLIs would
		// list the developer's own VMs, and some hang without their app's
		// state under the real HOME (orbctl waited out its 20s timeout).
		"AUTHAMA_VM_HYPERVISORS=lima",
		"AUTHAMA_NO_DOTENV=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"NO_COLOR=1",
	)
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	return &qaBox{t: t, bin: bin, home: home, env: env, dir: work}
}

type qaResult struct {
	out, err string
	code     int
}

func (b *qaBox) run(stdin string, args ...string) qaResult {
	b.t.Helper()
	cmd := exec.Command(b.bin, args...)
	cmd.Env, cmd.Dir = b.env, b.dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, errBuf strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		b.t.Fatalf("%v: %v", args, err)
	}
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		b.t.Fatalf("%v hung for 60s:\n%s%s", args, out.String(), errBuf.String())
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		b.t.Fatalf("%v: %v", args, err)
	}
	r := qaResult{out.String(), errBuf.String(), code}
	if strings.Contains(r.err, "panic:") || strings.Contains(r.err, "goroutine ") {
		b.t.Errorf("%v panicked:\n%s", args, r.err)
	}
	return r
}

var subcommandLine = regexp.MustCompile(`^    ([a-z][a-z0-9-]*)\s{2,}\S`)

// commands walks the tree through help: every path the root's help reaches.
func (b *qaBox) commands(path []string, into *[][]string) {
	b.t.Helper()
	r := b.run("", append(slices.Clone(path), "--help")...)
	name := strings.Join(append([]string{"authama"}, path...), " ")
	if r.code != 0 {
		b.t.Errorf("%s --help exited %d: %s", name, r.code, r.err)
		return
	}
	for _, section := range []string{"NAME\n    " + name + " - ", "SYNOPSIS\n", "DESCRIPTION\n", "SEE ALSO\n"} {
		if !strings.Contains(r.out, section) {
			b.t.Errorf("%s --help has no %q:\n%s", name, strings.TrimSpace(section), r.out)
		}
	}
	if strings.TrimSpace(r.err) != "" {
		b.t.Errorf("%s --help wrote to stderr: %s", name, r.err)
	}
	// The NAME line carries a one-line summary after the dash.
	if _, summary, _ := strings.Cut(strings.SplitN(r.out, "\n", 3)[1], " - "); strings.TrimSpace(summary) == "" {
		b.t.Errorf("%s --help has no summary on its NAME line", name)
	}
	*into = append(*into, path)
	in := false
	for _, line := range strings.Split(r.out, "\n") {
		switch {
		case line == "COMMANDS":
			in = true
			continue
		case strings.TrimSpace(line) == "":
			in = false
		}
		if m := subcommandLine.FindStringSubmatch(line); in && m != nil && m[1] != "help" && m[1] != "completion" {
			b.commands(append(slices.Clone(path), m[1]), into)
		}
	}
}

// Every command answers --help cleanly, and refuses a flag it does not take
// with a usage error naming it, never a panic.
func TestQAEveryCommandHelpsAndRefusesABadFlag(t *testing.T) {
	b := newQABox(t)
	var all [][]string
	b.commands(nil, &all)
	if len(all) < 150 {
		t.Fatalf("help reached only %d commands", len(all))
	}
	for _, path := range all {
		if len(path) == 0 {
			continue
		}
		args := append(slices.Clone(path), "--qa-no-such-flag")
		r := b.run("", args...)
		if r.code == 0 {
			t.Errorf("%v took an unknown flag", path)
			continue
		}
		if !strings.Contains(r.err, "unknown flag") && !strings.Contains(r.err, "qa-no-such-flag") {
			t.Errorf("%v: an unknown flag says %q", path, strings.TrimSpace(r.err))
		}
	}
	t.Logf("%d commands checked", len(all))
}

// qaStep is one command the walk runs: the exit code it must give (or -1
// for "not 0"), and what its output must say.
type qaStep struct {
	args  []string
	stdin string
	code  int
	want  []string // each in stdout+stderr
	not   []string // none in stdout+stderr
}

func ok(args ...string) qaStep    { return qaStep{args: args} }
func fails(args ...string) qaStep { return qaStep{args: args, code: -1} }
func (s qaStep) says(w ...string) qaStep {
	s.want = append(s.want, w...)
	return s
}
func (s qaStep) never(w ...string) qaStep {
	s.not = append(s.not, w...)
	return s
}
func (s qaStep) in(stdin string) qaStep {
	s.stdin = stdin
	return s
}

func (b *qaBox) step(s qaStep) qaResult {
	b.t.Helper()
	r := b.run(s.stdin, s.args...)
	both := r.out + r.err
	switch {
	case s.code == -1 && r.code == 0:
		b.t.Errorf("authama %s should fail, exited 0:\n%s", strings.Join(s.args, " "), both)
	case s.code >= 0 && r.code != s.code:
		b.t.Errorf("authama %s exited %d, want %d:\n%s", strings.Join(s.args, " "), r.code, s.code, both)
	}
	for _, w := range s.want {
		if !strings.Contains(both, w) {
			b.t.Errorf("authama %s does not say %q:\n%s", strings.Join(s.args, " "), w, both)
		}
	}
	for _, w := range s.not {
		if strings.Contains(both, w) {
			b.t.Errorf("authama %s says %q:\n%s", strings.Join(s.args, " "), w, both)
		}
	}
	return r
}

// closedPort is a localhost port nothing listens on, so a probe fails fast.
func closedPort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("cannot bind localhost")
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return fmt.Sprint(port)
}

// The walk: the commands and their main flags, in the order a person uses
// them, each checked for its exit code and what it says.
func TestQACommandWalk(t *testing.T) {
	b := newQABox(t)
	port := closedPort(t)
	out := filepath.Join(b.home, "out")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	sel := filepath.Join(out, "sel.json")
	if err := os.WriteFile(sel, []byte(`{"version":1,"grain":"host","where":{"tag":["prod"]},"columns":[{"field":"alias"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	steps := []qaStep{
		ok("version").says("authama"),
		ok("init", "--yes"),
		ok("settings", "path"),
		ok("settings", "path", "--json").says("{"),
		ok("settings", "show"),
		ok("settings", "show", "--effective"),
		ok("settings", "show", "--json").says("{"),
		ok("settings", "validate"),
		ok("ssh", "profile", "add", "work", "--code", "wk", "--category", "clients"),
		ok("ssh", "profile", "add", "personal", "--key-scope", "shared", "--key-name", "main"),
		fails("ssh", "profile", "add", "work"),
		fails("ssh", "profile", "add", "../evil"),
		ok("ssh", "host", "add", "work", "web", "--hostname", "127.0.0.1", "--port", port, "--user", "deploy", "--tag", "prod"),
		fails("ssh", "host", "add", "work", "db", "--hostname", "127.0.0.1", "--user", "deploy", "--role", "db").says("leave out <alias>"),
		ok("ssh", "host", "add", "work", "db", "--hostname", "127.0.0.1", "--port", port, "--user", "deploy", "--tag", "db"),
		ok("ssh", "host", "add", "personal", "box", "--hostname", "127.0.0.1", "--port", port, "--user", "me"),
		fails("ssh", "host", "add", "nope", "x", "--hostname", "127.0.0.1", "--user", "u"),
		fails("ssh", "host", "add", "work", "web", "--hostname", "127.0.0.1", "--user", "u"),
		ok("ssh", "host", "edit", "work", "db", "--tag", "data", "--untag", "db"),
		ok("ssh", "host", "apply", "work", "api", "127.0.0.1:"+port, "deploy", "app", "--dry-run"),
		ok("ssh", "host", "apply", "work", "api", "127.0.0.1:"+port, "deploy", "app", "--yes", "--no-pin"),
		ok("ssh", "key", "add", "work", "spare", "--rotate-after-days", "90"),
		ok("ssh", "reconcile", "--dry-run"),
		ok("ssh", "reconcile", "--yes", "--no-pin"),
		ok("ssh", "reconcile", "--yes", "--no-pin"),
		ok("ssh", "config", "check"),
		ok("ssh", "config", "show", "web").says("127.0.0.1"),
		ok("ssh", "config", "render", "--dry-run"),
		ok("ssh", "config", "export", "--flat", "--out", filepath.Join(out, "ssh_config")),
		ok("ssh", "list").says("web", "db", "box"),
		ok("ssh", "list", "--profile", "work").says("web").never("box"),
		ok("ssh", "list", "--tag", "prod").says("web").never("box"),
		ok("ssh", "list", "--csv").says(","),
		ok("ssh", "show", "web"),
		ok("ssh", "show", "work"),
		ok("ssh", "view", "work"),
		ok("ssh", "validate"),
		ok("ssh", "diff"),
		ok("ssh", "expiry"),
		ok("ssh", "audit"),
		ok("ssh", "audit", "--csv"),
		ok("ssh", "key", "list"),
		ok("ssh", "key", "list", "--json").says("["),
		ok("ssh", "key", "list", "--csv"),
		ok("ssh", "key", "list", "--new"),
		ok("ssh", "key", "ack", "--all"),
		ok("ssh", "key", "ack", "--category", "clients"),
		ok("ssh", "key", "lifetime", "work/spare", "quarterly", "--yes"),
		ok("ssh", "key", "regen-pub", "--all", "--dry-run"),
		ok("ssh", "key", "regen-pub", "--all", "--yes"),
		ok("ssh", "naming", "show"),
		ok("ssh", "naming", "check"),
		ok("ssh", "net").says("web"),
		ok("ssh", "providers"),
		ok("ssh", "providers", "--export"),
		ok("ssh", "clean", "--dry-run"),
		ok("ssh", "config", "adopt", "--dry-run"),
		ok("ssh", "structure", "list"),
		ok("ssh", "structure", "list", "--json"),
		ok("ssh", "structure", "check", "--quiet"),
		ok("ssh", "snapshots", "list"),
		ok("ssh", "snapshots", "list", "--csv"),
		ok("ssh", "snapshots", "prune", "--keep", "5"),
		ok("ssh", "vm", "list"),
		ok("ssh", "git", "sign", "status"),
		ok("ssh", "git", "sign", "status", "--json"),
		ok("ssh", "git", "status", b.dir),
		fails("ssh", "git", "resolve", "acme/app", "--profile", "work").says("no github.com host"),
		fails("ssh", "git", "clone", "acme/app", "--profile", "work", "--dry-run").says("no github.com host"),
		ok("ssh", "termius", "keys", "--help"),
		ok("ssh", "rotate", "work/spare", "--check"),
		fails("ssh", "rotate").says("name keys"),
		ok("history", "--command", "rotate").says("dry-run").never("error"),
		fails("ssh", "notify", "doctor", "--title", "x").says("unknown flag"),
		ok("ssh", "knownhosts", "init", "--help"),
		fails("ssh", "key", "reserve", filepath.Join(b.home, ".ssh", "id_qa_missing"), "--dry-run").says("no key file ~/.ssh/id_qa_missing").never("lstat"),
		ok("history", "--command", "token set").says("no operations match").never("recorded yet"),
		ok("history"),
		ok("history", "--json"),
		ok("history", "--csv"),
		ok("history", "--command", "ssh host add").says("host add"),
		ok("events"),
		ok("events", "--json"),
		ok("events", "--csv"),
		// The unwired spare key fails the verdict, by design.
		fails("doctor").says("unwired", "work/spare"),
		fails("doctor", "--json").says(`"ok": false`, `"dangling_keys"`, "work/spare"),
		ok("repair", "--check"),
		ok("checklist"),
		ok("checklist", "--json"),
		ok("companion", "info"),
		ok("companion", "info", "--json"),
		ok("export", "hosts").says("web"),
		ok("export", "hosts", "--format", "json").says("["),
		ok("export", "hosts", "--format", "csv").says("web"),
		ok("export", "hosts", "--format", "termius").says("web"),
		ok("export", "hosts", "--format", "termius", "--termius-groups", "category/profile").says("clients/work"),
		ok("export", "keys", "--format", "json"),
		ok("export", "profiles"),
		ok("export", "history", "--format", "csv"),
		ok("export", "events"),
		ok("export", "snapshots"),
		ok("export", "tokens"),
		ok("export", "deployments"),
		ok("export", "pack", "--out", filepath.Join(out, "pack.zip")),
		ok("export", "pack", "--dry-run"),
		fails("export", "selections", "save", "prd", "--from", filepath.Join(out, "missing.json")),
		ok("export", "selections", "save", "prd", "--from", sel, "--yes"),
		ok("export", "selections", "list").says("prd"),
		ok("export", "selections", "list", "--json").says("prd"),
		ok("export", "selections", "show", "prd"),
		ok("export", "prd").says("web"),
		ok("export", "schedule", "add", "prd", "--format", "json", "--to", out),
		ok("export", "schedule", "list").says("prd"),
		ok("export", "schedule", "run", "--quiet"),
		ok("export", "schedule", "remove", "prd"),
		ok("export", "selections", "delete", "prd", "--yes"),
		ok("token", "examples"),
		ok("token", "examples", "--json"),
		ok("token", "list"),
		ok("token", "list", "--json"),
		ok("token", "status"),
		ok("token", "status", "--json"),
		ok("token", "audit"),
		ok("token", "providers"),
		ok("token", "shell-aliases", "--print"),
		ok("setup", "export", "--out", filepath.Join(out, "setup.json")),
		ok("setup", "import", filepath.Join(out, "setup.json"), "--dry-run"),
		ok("api", "token", "list"),
		fails("api", "token", "create", "qa", "--expires", "30d").says("--expires"),
		ok("api", "token", "create", "qa", "--expires", "30").says("qa"),
		ok("api", "token", "list").says("qa"),
		ok("api", "token", "revoke", "qa"),
		ok("webhook", "list"),
		ok("owner", "status"),
		ok("man", filepath.Join(out, "man")),
		ok("serve", "--status"),
		ok("ssh", "host", "delete", "work", "api", "--purge", "--no-key-backup", "--yes"),
		ok("ssh", "key", "delete", "work/spare", "--purge", "--no-key-backup", "--yes"),
		ok("ssh", "profile", "edit", "personal", "--key-comment", "{profile}/{alias}"),
		ok("ssh", "profile", "delete", "personal", "--purge", "--no-key-backup", "--yes"),
		ok("ssh", "config", "check"),
	}
	for _, s := range steps {
		b.step(s)
	}
	// The walk's own manifest is valid after all of it.
	b.step(ok("ssh", "validate"))
	b.step(ok("doctor"))
}
