package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/renderer"
	"github.com/simtabi/authama/src/internal/util/encode"
)

// clientsHome is a home with each kind of host the client exports treat
// differently: plain, with a port and tags, behind a ProxyCommand, and git.
func clientsHome(t *testing.T) {
	t.Helper()
	editHome(t)
	prev := renderer.SelfPath
	renderer.SelfPath = func() string { return "/usr/local/bin/authama" }
	t.Cleanup(func() { renderer.SelfPath = prev })
	run(t, "profile", "add", "work")
	run(t, "profile", "add", "simtabi")
	run(t, "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "-p", "2222", "--tag", "prd", "--tag", "a,b")
	run(t, "host", "add", "work", "db1", "-H", "db1.example", "-u", "deploy")
	run(t, "host", "add", "simtabi", "oribi-web", "-H", "ssh-web-00.simtabi.com", "-u", "adm-imani", "--proxy", "cloudflared")
	run(t, "host", "add", "simtabi", "gh", "-H", "github.com", "-u", "git")
}

// filesOf reads a file the pipeline wrote as the files it holds: a zip's
// entries, or the file itself.
func filesOf(t *testing.T, path string, f encode.Format) map[string][]byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a test temp path
	if err != nil {
		t.Fatal(err)
	}
	if f.Extension != "zip" {
		return map[string][]byte{"": b}
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, e := range zr.File {
		r, err := e.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		_ = r.Close()
		out[e.Name] = buf.Bytes()
	}
	return out
}

// Every `ssh host export` client, through `authama export hosts --format`,
// writes the bytes the old command writes, with and without git hosts.
// The old command keeps working: it is the reference here.
func TestExportHostsInEveryClientFormatIsTheOldCommandsFile(t *testing.T) {
	clientsHome(t)
	checked := 0
	for _, client := range encode.ClientNames() {
		name := client
		if client == "csv" {
			name = encode.ClientCSV
		}
		f, err := encode.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, git := range [][]string{nil, {"--include-git"}} {
			dir := t.TempDir()
			old := filepath.Join(dir, "old")
			run(t, append([]string{"ssh", "host", "export", client, "--out", old}, git...)...)
			target := filepath.Join(dir, "new")
			run(t, append([]string{"export", "hosts", "--format", name, "--out", target}, git...)...)
			got := filesOf(t, target, f)
			entries, err := os.ReadDir(old)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != len(got) {
				t.Fatalf("%s %v: old wrote %d files, new %d", name, git, len(entries), len(got))
			}
			for _, e := range entries {
				want, _ := os.ReadFile(filepath.Join(old, e.Name())) //nolint:gosec // test temp path
				key := e.Name()
				if f.Extension != "zip" {
					key = ""
				}
				if !bytes.Equal(got[key], want) {
					t.Errorf("%s %v: %s differs:\n--- export hosts\n%s\n--- ssh host export\n%s", name, git, e.Name(), got[key], want)
				}
				checked++
			}
		}
	}
	if checked < 2*len(encode.ClientNames()) {
		t.Fatalf("compared %d files", checked)
	}
}

// The Termius format is `ssh termius export`'s CSV, and ssh_config is
// `ssh config export --flat`, byte for byte, when every host is kept.
func TestExportHostsAsTermiusAndSSHConfigIsTheOldCommandsFile(t *testing.T) {
	clientsHome(t)
	dir := t.TempDir()
	for _, c := range []struct {
		old, new []string
	}{
		{[]string{"ssh", "termius", "export"}, []string{"export", "hosts", "--format", "termius"}},
		{[]string{"ssh", "termius", "export", "--include-git"}, []string{"export", "hosts", "--format", "termius", "--include-git"}},
		{[]string{"ssh", "config", "export", "--flat"}, []string{"export", "hosts", "--format", "ssh_config"}},
	} {
		oldF, newF := filepath.Join(dir, "old"), filepath.Join(dir, "new")
		run(t, append(c.old, "--out", oldF)...)
		run(t, append(c.new, "--out", newF)...)
		want, _ := os.ReadFile(oldF) //nolint:gosec // test temp path
		got, _ := os.ReadFile(newF)  //nolint:gosec // test temp path
		if len(want) == 0 || !bytes.Equal(got, want) {
			t.Errorf("%v differs from %v:\n--- new\n%s\n--- old\n%s", c.new, c.old, got, want)
		}
	}
}

// Filters choose the hosts: ssh_config of one profile is that profile's
// blocks and Host *, nothing else; a client format leaves out a host it
// cannot reach and says so on stderr, keeping stdout a clean file.
func TestExportHostsFiltersTheHostsAndReportsWhatItLeftOut(t *testing.T) {
	clientsHome(t)
	out := run(t, "export", "hosts", "--profile", "work", "--format", "ssh_config")
	if !strings.Contains(out, "Host web1\n") || !strings.Contains(out, "Host db1\n") || strings.Contains(out, "Host oribi-web") ||
		!strings.Contains(out, "Host *\n") {
		t.Errorf("ssh_config of work:\n%s", out)
	}
	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"export", "hosts", "--format", "putty"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte{0xFF, 0xFE}) {
		t.Errorf("stdout is not the .reg file: %q", stdout.Bytes()[:min(20, stdout.Len())])
	}
	if s := stderr.String(); !strings.Contains(s, "skipped: oribi-web") || !strings.Contains(s, "skipped: gh") {
		t.Errorf("stderr does not say what was left out:\n%s", s)
	}
	// --sort orders the hosts as the selection does; without it, manifest order.
	sorted := run(t, "export", "hosts", "--format", "windows-terminal", "--sort", "alias", "--include-git")
	plain := run(t, "export", "hosts", "--format", "windows-terminal", "--include-git")
	if strings.Index(sorted, "db1") > strings.Index(sorted, "web1") || strings.Index(plain, "web1") > strings.Index(plain, "db1") {
		t.Errorf("order:\n--- sorted\n%s--- plain\n%s", sorted, plain)
	}
	if _, err := runErr(t, "export", "keys", "--format", "putty"); err == nil || !strings.Contains(err.Error(), "export hosts instead") {
		t.Errorf("a host format of keys: %v", err)
	}
	names := encode.HostNames()
	sort.Strings(names)
	help := run(t, "export", "--help")
	for _, n := range names {
		if !strings.Contains(help, n) {
			t.Errorf("export --help does not name %s", n)
		}
	}
}
