package importer

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/services/configsvc"
)

// messyConfig is a hand-built ~/.ssh/config of the kind the documented
// onboarding meets: defaults at the top, plain hosts, a Match block, a
// wildcard block, and a Host * of defaults at the end.
const messyConfig = `# my setup
Host *.lab
    StrictHostKeyChecking accept-new

Host web
    HostName web.example
    User deploy
    Port 2222

Host db
    HostName db.example
    User ops
    ServerAliveInterval 15

Match host *.corp exec "test -n x"
    ForwardAgent no

Host *
    ServerAliveInterval 45
    AddKeysToAgent no
`

func countHostLines(content, alias string) int {
	n := 0
	for _, ln := range strings.Split(content, "\n") {
		f := strings.Fields(ln)
		if len(f) >= 2 && strings.EqualFold(f[0], "Host") && slices.Contains(f[1:], alias) {
			n++
		}
	}
	return n
}

// The documented onboarding is `import -y` then `reconcile`. Import used to
// copy each host into the manifest and leave its block where it was, so the
// render that followed defined every host twice and doctor failed on the
// first run (review R-BUG-09). Import now moves each block it imports out of
// the config and renders the managed block in its place: one definition of
// every host, Match and wildcard blocks untouched, nothing shadowed.
func TestImportThenRenderDefinesEachHostOnce(t *testing.T) {
	im, p, cfgPath := setupImport(t, messyConfig)
	res, err := im.Run(cfgPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Moved, ",") != "db,web" {
		t.Errorf("Moved = %v, want db and web", res.Moved)
	}
	m := mustLoad(t, p)
	svc := configsvc.New(p.SSHDir, m, false)
	// What reconcile does to the config: render it.
	if _, err := svc.Write(false); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(cfgPath)
	got := string(body)
	for _, alias := range []string{"web", "db"} {
		if n := countHostLines(got, alias); n != 1 {
			t.Errorf("Host %s is defined %d times after import and render:\n%s", alias, n, got)
		}
	}
	for _, kept := range []string{"Host *.lab", `Match host *.corp exec "test -n x"`, "ForwardAgent no", "# my setup"} {
		if !strings.Contains(got, kept) {
			t.Errorf("import lost %q from the user's config:\n%s", kept, got)
		}
	}
	chk, err := svc.Check(false)
	if err != nil {
		t.Fatal(err)
	}
	if !chk.InSync() {
		t.Errorf("after import and render the config is not clean (doctor would fail):\n%s\n---\n%s", chk.Format(), got)
	}
	// The managed block took the place of the first block it replaced: the
	// user's Host * of defaults is still read after the hosts.
	header := strings.Index(got, renderer.ManagedHeader)
	userStar := strings.LastIndex(got, "Host *\n    ServerAliveInterval 45")
	if header < 0 || userStar < header || strings.Index(got, "Host *.lab") > header {
		t.Errorf("the managed block is not where the first moved block was:\n%s", got)
	}
	// The user's Host * values for options authama's Host * sets are kept.
	if v := m.Defaults.GlobalOptions.Get("ServerAliveInterval"); v != "45" {
		t.Errorf("global ServerAliveInterval = %q, want the user's 45", v)
	}
	if v := m.Defaults.GlobalOptions.Get("AddKeysToAgent"); v != "no" {
		t.Errorf("global AddKeysToAgent = %q, want the user's no", v)
	}
	// Nothing is lost: the original is backed up.
	saved, err := os.ReadFile(filepath.Join(res.Backup, "config"))
	if err != nil || string(saved) != messyConfig {
		t.Errorf("the backup does not hold the original config: %v", err)
	}
}

// A dry run says what it would move and writes nothing.
func TestImportDryRunMovesNothing(t *testing.T) {
	im, _, cfgPath := setupImport(t, messyConfig)
	res, err := im.Run(cfgPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Moved) != 2 || res.Backup != "" {
		t.Errorf("dry run: Moved=%v Backup=%q", res.Moved, res.Backup)
	}
	if b, _ := os.ReadFile(cfgPath); string(b) != messyConfig {
		t.Errorf("a dry run edited the config:\n%s", b)
	}
}

// A block import cannot reproduce leaves its host the user's: not imported,
// and the block untouched, so there is still one definition of it.
func TestABlockTheManifestCannotHoldKeepsItsHostTheUsers(t *testing.T) {
	cfg := `Host jump
    HostName jump.example
    ProxyCommand nc %h %p | tee /tmp/log

Host pair alpha*
    HostName pair.example

Host mixed
    HostName mixed.example
    UserKnownHostsFile ~/.ssh/known_hosts_mixed

Host plain
    HostName plain.example
`
	im, p, cfgPath := setupImport(t, cfg)
	res, err := im.Run(cfgPath, false)
	if err != nil {
		t.Fatal(err)
	}
	m := mustLoad(t, p)
	if h, _ := hostIn(m, "plain"); h.Alias != "plain" {
		t.Fatalf("plain was not imported: %+v", m.Profiles)
	}
	for _, alias := range []string{"jump", "pair", "mixed"} {
		if h, _ := hostIn(m, alias); h.Alias != "" {
			t.Errorf("%s was imported though its block stays", alias)
		}
		found := false
		for _, k := range res.Kept {
			found = found || strings.HasPrefix(k, alias+" - ")
		}
		if !found {
			t.Errorf("Kept = %v does not say why %s stays", res.Kept, alias)
		}
	}
	body, _ := os.ReadFile(cfgPath)
	for _, kept := range []string{"ProxyCommand nc %h %p | tee /tmp/log", "Host pair alpha*", "known_hosts_mixed"} {
		if !strings.Contains(string(body), kept) {
			t.Errorf("a kept block was edited, %q is gone:\n%s", kept, body)
		}
	}
	if n := countHostLines(string(body), "plain"); n != 1 {
		t.Errorf("Host plain defined %d times", n)
	}
}

// Running the onboarding again changes nothing: the second import finds the
// hosts in the managed block, moves nothing, and leaves the file as it was.
func TestImportIsIdempotent(t *testing.T) {
	im, p, cfgPath := setupImport(t, messyConfig)
	if _, err := im.Run(cfgPath, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(cfgPath)
	res, err := New(p, false).Run(cfgPath, false)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(cfgPath)
	if len(res.Moved) != 0 || string(first) != string(second) {
		t.Errorf("a second import moved %v or changed the config:\n%s\n---\n%s", res.Moved, first, second)
	}
	if h, _ := hostIn(mustLoad(t, p), "web"); h.Port != 2222 {
		t.Errorf("re-import lost web: %+v", h)
	}
}

// A host in a file the config Includes is moved out of that file, and the
// managed block goes where the Include was read.
func TestImportMovesABlockOutOfAnIncludedFile(t *testing.T) {
	im, p, cfgPath := setupImport(t, "Include config.d/*\n\nHost *\n    ServerAliveInterval 20\n")
	inc := filepath.Join(p.SSHDir, "config.d", "work")
	if err := os.MkdirAll(filepath.Dir(inc), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inc, []byte("Host work\n    HostName work.example\n    User me\n\nHost *.work\n    User team\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := im.Run(cfgPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(res.Moved, "work") {
		t.Fatalf("Moved = %v", res.Moved)
	}
	incBody, _ := os.ReadFile(inc)
	if strings.Contains(string(incBody), "Host work\n") || !strings.Contains(string(incBody), "Host *.work") {
		t.Errorf("the included file was not edited as it should be:\n%s", incBody)
	}
	root, _ := os.ReadFile(cfgPath)
	if countHostLines(string(root), "work") != 1 ||
		strings.Index(string(root), renderer.ManagedHeader) > strings.Index(string(root), "Include config.d/*") {
		t.Errorf("the managed block is not rendered where the Include was:\n%s", root)
	}
}

// Importing another machine's copy edits nothing: that config is not one ssh
// reads here.
func TestImportOfAnotherMachinesCopyEditsNothing(t *testing.T) {
	im, _, _ := setupImport(t, "")
	other := filepath.Join(t.TempDir(), "laptop")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(other, "config")
	if err := os.WriteFile(src, []byte(messyConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := im.Run(other, false)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(src); string(b) != messyConfig || len(res.Moved) != 0 || len(res.Kept) != 0 {
		t.Errorf("an import of another machine's copy edited it: moved=%v kept=%v", res.Moved, res.Kept)
	}
}

// A profile directory that is a symlink is refused before anything follows
// it: MkdirAll and the 0700 chmod used to land on the link's target, wherever
// that was (review A-SEC-15).
func TestAdoptRefusesASymlinkedProfileDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	im, p, cfgPath := setupImport(t, `Host gh
    HostName github.com
    IdentityFile `+filepath.Join(t.TempDir(), "loose_key")+`
`)
	mintKey(t, identityIn(t, cfgPath))
	target := t.TempDir()
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p.SSHDir, "profiles"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(p.SSHDir, "profiles", "imported")); err != nil {
		t.Skip("cannot make a symlink here:", err)
	}
	res, err := im.Run(cfgPath, false)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("the link's target was chmodded to %04o", fi.Mode().Perm())
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Errorf("a key was written through the link: %v", entries)
	}
	if res.Adopted != 0 || !strings.Contains(strings.Join(res.Skipped, "\n"), "symbolic link") {
		t.Errorf("Adopted=%d Skipped=%v, want the refusal reported", res.Adopted, res.Skipped)
	}
}
