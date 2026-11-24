package sshcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSSHDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ssh := filepath.Join(dir, ".ssh")
	if err := os.Mkdir(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)
	return ssh
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFollowsRelativeGlobsAndTildeIncludes(t *testing.T) {
	ssh := testSSHDir(t)
	write(t, filepath.Join(ssh, "config.d", "a.conf"), "Host a\n  HostName a.example\n")
	write(t, filepath.Join(ssh, "config.d", "b.conf"), "Host b\n  HostName b.example\n  IdentityFile ~/.ssh/id_b\n")
	// A relative Include is resolved against ~/.ssh, not the including file.
	write(t, filepath.Join(ssh, "config.d", "more", "c.conf"), "Include extra.conf\nHost c\n  HostName c.example\n")
	write(t, filepath.Join(ssh, "extra.conf"), "Host extra\n  HostName extra.example\n")
	write(t, filepath.Join(ssh, "config"),
		"Include config.d/*.conf\nInclude ~/.ssh/config.d/more/c.conf\nHost root\n  HostName root.example\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range tree.Hosts {
		if h.Alias != "" {
			got[h.Alias] = h.File
		}
	}
	for _, name := range []string{"a", "b", "c", "extra", "root"} {
		if got[name] == "" {
			t.Errorf("host %s not found in %#v", name, got)
		}
	}
	if !strings.HasSuffix(got["extra"], "extra.conf") {
		t.Errorf("extra should come from ~/.ssh/extra.conf (relative Include from a nested file), got %s", got["extra"])
	}
	if filepath.Base(got["root"]) != "config" {
		t.Errorf("root host file = %s", got["root"])
	}
	if len(tree.Warnings) != 0 {
		t.Errorf("warnings: %+v", tree.Warnings)
	}
	text := tree.Format()
	if !strings.Contains(text, "Host a") || !strings.Contains(text, "Include") {
		t.Errorf("format missing hosts or includes:\n%s", text)
	}
}

func TestLoadFollowsStarStarAndMissingAndMatch(t *testing.T) {
	ssh := testSSHDir(t)
	write(t, filepath.Join(ssh, "nested", "d1", "x.conf"), "Host nest\n  HostName nest.example\n")
	write(t, filepath.Join(ssh, "config"),
		"Include nested/**/*.conf\n"+
			"Include missing.conf\n"+
			"Match host orb\n"+
			"Include config.d/orb.conf\n"+
			"Host visible\n  HostName visible.example\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string]Host{}
	for _, h := range tree.Hosts {
		if h.Alias != "" {
			aliases[h.Alias] = h
		}
	}
	if aliases["nest"].File == "" {
		t.Fatal("** glob did not reach nested/d1/x.conf")
	}
	if aliases["visible"].File == "" {
		t.Fatal("Host visible missing")
	}
	var sawMissing bool
	for _, w := range tree.Warnings {
		if w.Kind == WarnMissing && strings.Contains(w.Message, "missing.conf") {
			sawMissing = true
		}
	}
	if !sawMissing {
		t.Errorf("missing file produced no warning: %+v", tree.Warnings)
	}
	// The orb include is missing too (config.d/orb.conf is not there) and is
	// inside a Match; both should be warned, not fatal.
	if tree.Root == nil || len(tree.Root.Includes) == 0 {
		t.Fatal("expected include children")
	}
}

func TestLoadCycleAndDepthAreWarnedAndTerminate(t *testing.T) {
	ssh := testSSHDir(t)
	a := filepath.Join(ssh, "a.conf")
	b := filepath.Join(ssh, "b.conf")
	write(t, a, "Include b.conf\nHost a\n  HostName a.example\n")
	write(t, b, "Include a.conf\nHost b\n  HostName b.example\n")
	write(t, filepath.Join(ssh, "config"), "Include a.conf\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	var cycle bool
	for _, w := range tree.Warnings {
		if w.Kind == WarnCycle {
			cycle = true
		}
	}
	if !cycle {
		t.Errorf("cycle produced no warning: %+v\n%s", tree.Warnings, tree.Format())
	}
	got := map[string]bool{}
	for _, h := range tree.Hosts {
		got[h.Alias] = true
	}
	if !got["a"] || !got["b"] {
		t.Errorf("cycle dropped hosts: %v", got)
	}

	// A chain deeper than MaxIncludeDepth is skipped with a warning.
	prev := filepath.Join(ssh, "config")
	// Rebuild a long chain from a fresh dir so the cycle fixture doesn't
	// interfere.
	deep := testSSHDir(t)
	write(t, filepath.Join(deep, "config"), "Include d0.conf\n")
	for i := 0; i <= MaxIncludeDepth+2; i++ {
		next := filepath.Join(deep, "d"+itoa(i)+".conf")
		body := "Host d" + itoa(i) + "\n  HostName d.example\n"
		if i < MaxIncludeDepth+2 {
			body = "Include d" + itoa(i+1) + ".conf\n" + body
		}
		write(t, next, body)
		_ = prev
	}
	tree, err = LoadUser(deep)
	if err != nil {
		t.Fatal(err)
	}
	var depth bool
	for _, w := range tree.Warnings {
		if w.Kind == WarnDepth {
			depth = true
		}
	}
	if !depth {
		t.Errorf("depth produced no warning:\n%s", tree.Format())
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var d []byte
	for i > 0 {
		d = append([]byte{byte('0' + i%10)}, d...)
		i /= 10
	}
	return string(d)
}

func TestManagedMarksRootAndHeaderFilesOnly(t *testing.T) {
	ssh := testSSHDir(t)
	foreign := filepath.Join(t.TempDir(), "orb", "config")
	write(t, foreign, "Host orb\n  HostName 127.0.0.1\n")
	write(t, filepath.Join(ssh, "profiles", "orgs", "config"),
		"# Managed by authama - do not edit (run: authama ssh config render)\n"+
			"# Category: orgs\nHost gh\n  HostName github.com\n")
	write(t, filepath.Join(ssh, "config"),
		"Include "+foreign+"\n"+
			"# Managed by authama - do not edit (run: authama ssh config render)\n"+
			"Match all\n"+
			"Include profiles/orgs/config\n"+
			"# End of authama-managed block - content outside it is preserved\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Root == nil || !tree.Root.Managed {
		t.Fatal("root ~/.ssh/config should be managed")
	}
	var sawForeign, sawCat bool
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Path == foreign {
			sawForeign = true
			if n.Managed {
				t.Error("a foreign Include must not be marked managed")
			}
		}
		if strings.HasSuffix(n.Path, filepath.Join("profiles", "orgs", "config")) && n.Managed {
			sawCat = true
		}
		for _, c := range n.Includes {
			walk(c)
		}
	}
	walk(tree.Root)
	if !sawForeign {
		t.Error("foreign include was not in the tree")
	}
	if !sawCat {
		t.Error("managed category config was not marked managed")
	}
	for _, h := range tree.Hosts {
		if h.Alias == "orb" && h.Managed {
			t.Error("Host orb in a foreign file was marked managed")
		}
		if h.Alias == "gh" && !h.Managed {
			t.Error("Host gh in a managed category file was not marked managed")
		}
	}
}

func TestExpandMissingAndAbsolute(t *testing.T) {
	ssh := testSSHDir(t)
	p := filepath.Join(ssh, "only.conf")
	write(t, p, "Host x\n")
	got, miss := Expand("only.conf", ssh)
	if miss || len(got) != 1 || got[0] != p {
		t.Fatalf("Expand only.conf = %v missing=%v", got, miss)
	}
	if _, miss = Expand("nope.conf", ssh); !miss {
		t.Fatal("missing path should be reported")
	}
	got, miss = Expand(p, ssh)
	if miss || len(got) != 1 {
		t.Fatalf("absolute Expand = %v missing=%v", got, miss)
	}
}

func TestTokensKeepsQuotedAndEscapedSpaces(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want []string
	}{
		{`a b`, []string{"a", "b"}},
		{`"my hosts.conf" other`, []string{"my hosts.conf", "other"}},
		{`'my hosts.conf'`, []string{"my hosts.conf"}},
		{`path\ with\ spaces`, []string{"path with spaces"}},
		{`"mixed 'quotes'"`, []string{"mixed 'quotes'"}},
		// OpenSSH's argv_split: a backslash escapes only a quote, a backslash
		// or an unquoted space, and is kept before anything else, so a Windows
		// path keeps its separators. Eating every backslash broke Includes on
		// Windows: C:\Users\me became C:Usersme.
		{`C:\Users\me\orb\config`, []string{`C:\Users\me\orb\config`}},
		{`"C:\Program Files\x\config"`, []string{`C:\Program Files\x\config`}},
		{`a\\b`, []string{`a\b`}},
		{`say\"hi\"`, []string{`say"hi"`}},
		{`odd\x`, []string{`odd\x`}},
		{"", nil},
	}
	for _, c := range cases {
		got := Tokens(c.in)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("Tokens(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestLoadQuotedIncludePathWithSpaces(t *testing.T) {
	ssh := testSSHDir(t)
	write(t, filepath.Join(ssh, "my hosts.conf"), "Host spaced\n  HostName spaced.example\n")
	write(t, filepath.Join(ssh, "config"), `Include "my hosts.conf"`+"\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	if !hasHost(tree, "spaced") {
		t.Fatalf("quoted Include did not load Host spaced; hosts=%v warnings=%v", hostAliases(tree), tree.Warnings)
	}
}

func TestLoadStripsBOMAndAcceptsCRLF(t *testing.T) {
	ssh := testSSHDir(t)
	body := "\uFEFFHost bom\r\n  HostName bom.example\r\nInclude crlf.conf\r\n"
	write(t, filepath.Join(ssh, "config"), body)
	write(t, filepath.Join(ssh, "crlf.conf"), "\uFEFFHost crlf\r\n  HostName crlf.example\r\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	if !hasHost(tree, "bom") || !hasHost(tree, "crlf") {
		t.Fatalf("BOM/CRLF load missed hosts: %v warnings=%v", hostAliases(tree), tree.Warnings)
	}
}

func TestLoadDirectoryIncludeIsWarnedNotWalked(t *testing.T) {
	ssh := testSSHDir(t)
	if err := os.Mkdir(filepath.Join(ssh, "just-a-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ssh, "just-a-dir", "hidden.conf"), "Host hidden\n  HostName hidden.example\n")
	write(t, filepath.Join(ssh, "config"), "Include just-a-dir\nHost top\n  HostName top.example\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	if hasHost(tree, "hidden") {
		t.Fatal("Include of a directory must not walk it (OpenSSH only accepts files)")
	}
	if !hasHost(tree, "top") {
		t.Fatal("Host after a directory Include should still be read")
	}
	var saw bool
	for _, w := range tree.Warnings {
		if w.Kind == WarnMissing && strings.Contains(w.Message, "just-a-dir") {
			saw = true
		}
	}
	if !saw {
		t.Errorf("directory Include produced no missing warning: %+v", tree.Warnings)
	}
}

func TestLoadSandboxRetargetsTildeSSH(t *testing.T) {
	realHome := t.TempDir()
	sandboxHome := t.TempDir()
	realSSH := filepath.Join(realHome, ".ssh")
	sandbox := filepath.Join(sandboxHome, ".ssh")
	write(t, filepath.Join(realSSH, "shared.conf"), "Host realhome\n  HostName real.example\n")
	write(t, filepath.Join(sandbox, "shared.conf"), "Host sandbox\n  HostName sandbox.example\n")
	write(t, filepath.Join(sandbox, "config"), "Include ~/.ssh/shared.conf\n")
	t.Setenv("HOME", realHome)
	t.Setenv("USERPROFILE", realHome)

	tree, err := LoadUser(sandbox)
	if err != nil {
		t.Fatal(err)
	}
	if hasHost(tree, "realhome") {
		t.Fatal("Include ~/.ssh/... escaped the sandbox into process $HOME/.ssh")
	}
	if !hasHost(tree, "sandbox") {
		t.Fatalf("sandbox Include ~/.ssh/shared.conf should resolve under the user SSH dir; hosts=%v warnings=%v", hostAliases(tree), tree.Warnings)
	}
}

func TestLoadSkipsOversizedFile(t *testing.T) {
	ssh := testSSHDir(t)
	big := filepath.Join(ssh, "huge.conf")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxConfigBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ssh, "config"), "Include huge.conf\nHost after\n  HostName after.example\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	if !hasHost(tree, "after") {
		t.Fatal("Host after an oversized Include should still be read")
	}
	var saw bool
	for _, w := range tree.Warnings {
		if w.Kind == WarnTooLarge {
			saw = true
		}
	}
	if !saw {
		t.Errorf("oversized file produced no too_large warning: %+v", tree.Warnings)
	}
}

func TestExpandLineTruncatesHugeGlob(t *testing.T) {
	ssh := testSSHDir(t)
	dir := filepath.Join(ssh, "many")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// One past the cap so expandGlob reports truncation.
	for i := 0; i < MaxGlobMatches+1; i++ {
		write(t, filepath.Join(dir, itoa(i)+".conf"), "Host h"+itoa(i)+"\n  HostName x.example\n")
	}
	files, missing, truncated := ExpandLine("many/*.conf", ssh)
	if !truncated {
		t.Fatal("expected truncated=true when the glob exceeds MaxGlobMatches")
	}
	if len(files) != MaxGlobMatches {
		t.Errorf("got %d files, want %d", len(files), MaxGlobMatches)
	}
	if len(missing) != 0 {
		t.Errorf("missing=%v, want none", missing)
	}
}

func TestLoadDuplicateHostAcrossFiles(t *testing.T) {
	ssh := testSSHDir(t)
	write(t, filepath.Join(ssh, "a.conf"), "Host shared\n  HostName a.example\n")
	write(t, filepath.Join(ssh, "b.conf"), "Host shared\n  HostName b.example\n")
	write(t, filepath.Join(ssh, "config"), "Include a.conf\nInclude b.conf\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var files []string
	for _, h := range tree.Hosts {
		if h.Alias == "shared" {
			n++
			files = append(files, h.File)
		}
	}
	if n != 2 {
		t.Fatalf("duplicate Host shared should appear once per file, got %d from %v", n, files)
	}
}

func TestLoadMatchIncludeIsTaggedAndExecIsNotRun(t *testing.T) {
	ssh := testSSHDir(t)
	sentinel := filepath.Join(t.TempDir(), "ran")
	script := filepath.Join(t.TempDir(), "marker.sh")
	write(t, script, "#!/bin/sh\ntouch "+sentinel+"\n")
	if err := os.Chmod(script, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ssh, "inside.conf"), "Host inside\n  HostName inside.example\n")
	write(t, filepath.Join(ssh, "config"),
		"Match exec \""+script+"\"\n"+
			"Include inside.conf\n"+
			"Host after\n  HostName after.example\n")

	tree, err := LoadUser(ssh)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("Match exec was run while loading the config; authama must not execute it")
	}
	if !hasHost(tree, "inside") || !hasHost(tree, "after") {
		t.Fatalf("Match Include should be listed unconditionally; hosts=%v", hostAliases(tree))
	}
	var tagged bool
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if strings.HasSuffix(n.Path, "inside.conf") && n.FromMatch {
			tagged = true
		}
		for _, c := range n.Includes {
			walk(c)
		}
	}
	walk(tree.Root)
	if !tagged {
		t.Fatal("Include inside a Match should be tagged FromMatch")
	}
}

func hasHost(tree *Tree, alias string) bool {
	if tree == nil {
		return false
	}
	for _, h := range tree.Hosts {
		if h.Alias == alias {
			return true
		}
	}
	return false
}

func hostAliases(tree *Tree) []string {
	if tree == nil {
		return nil
	}
	var out []string
	for _, h := range tree.Hosts {
		if h.Alias != "" {
			out = append(out, h.Alias)
		}
	}
	return out
}
