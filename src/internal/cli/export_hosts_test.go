package cli

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/util/encode"
)

// clientsHome is a home with each kind of host the client exports treat
// differently: plain, with a port and tags, behind a ProxyCommand, and git.
func clientsHome(t *testing.T) {
	t.Helper()
	editHome(t)
	prev := renderer.SelfPath
	renderer.SelfPath = func() string { return "/usr/local/bin/authama" }
	t.Cleanup(func() { renderer.SelfPath = prev })
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "profile", "add", "simtabi")
	run(t, "ssh", "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy", "-p", "2222", "--tag", "prd", "--tag", "a,b")
	run(t, "ssh", "host", "add", "work", "db1", "-H", "db1.example", "-u", "deploy")
	run(t, "ssh", "host", "add", "simtabi", "oribi-web", "-H", "ssh-web-00.simtabi.com", "-u", "adm-imani", "--proxy", "cloudflared")
	run(t, "ssh", "host", "add", "simtabi", "gh", "-H", "github.com", "-u", "git")
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

// Every client format, through `authama export hosts --format`, writes the
// bytes renderer.RenderClientExport produces for the same hosts, with and
// without git hosts. That renderer is the one the golden files pin
// (renderer.TestClientExportsMatchTheirGoldenFiles), so the pipeline cannot
// drift from them.
func TestExportHostsInEveryClientFormatIsTheRenderersFile(t *testing.T) {
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
		for _, git := range []bool{false, true} {
			args := []string{"export", "hosts", "--format", name, "--out", filepath.Join(t.TempDir(), "new")}
			if git {
				args = append(args, "--include-git")
			}
			run(t, args...)
			got := filesOf(t, args[5], f)
			want := renderClients(t, client, git)
			if len(want.Files) != len(got) {
				t.Fatalf("%s git=%v: the renderer wrote %d files, export hosts %d", name, git, len(want.Files), len(got))
			}
			for _, wf := range want.Files {
				key := wf.Name
				if f.Extension != "zip" {
					key = ""
				}
				if !bytes.Equal(got[key], wf.Data) {
					t.Errorf("%s git=%v: %s differs:\n--- export hosts\n%s\n--- renderer\n%s", name, git, wf.Name, got[key], wf.Data)
				}
				checked++
			}
		}
	}
	if checked < 2*len(encode.ClientNames()) {
		t.Fatalf("compared %d files", checked)
	}
}

// renderClients is what renderer.RenderClientExport writes for the home
// clientsHome built.
func renderClients(t *testing.T, client string, includeGit bool) *renderer.ClientExport {
	t.Helper()
	p, m, _, err := loadExport(newRootCmd())
	if err != nil {
		t.Fatal(err)
	}
	exp, err := renderer.RenderClientExport(m, exportPaths(p), client, renderer.TermiusOptions{IncludeGit: includeGit})
	if err != nil {
		t.Fatal(err)
	}
	return exp
}

// The Termius format is renderer.RenderTermiusCSV's CSV, and ssh_config is
// `ssh config export --flat`, byte for byte, when every host is kept.
func TestExportHostsAsTermiusAndSSHConfigIsTheRenderersFile(t *testing.T) {
	clientsHome(t)
	p, m, _, err := loadExport(newRootCmd())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, git := range []bool{false, true} {
		exp, err := renderer.RenderTermiusCSV(m, exportPaths(p), renderer.TermiusOptions{IncludeGit: git})
		if err != nil {
			t.Fatal(err)
		}
		newF := filepath.Join(dir, "termius")
		args := []string{"export", "hosts", "--format", "termius", "--out", newF}
		if git {
			args = append(args, "--include-git")
		}
		run(t, args...)
		got, _ := os.ReadFile(newF) //nolint:gosec // test temp path
		if exp.CSV == "" || string(got) != exp.CSV {
			t.Errorf("termius git=%v differs:\n--- export hosts\n%s\n--- renderer\n%s", git, got, exp.CSV)
		}
	}
	oldF, newF := filepath.Join(dir, "old"), filepath.Join(dir, "new")
	run(t, "ssh", "config", "export", "--flat", "--out", oldF)
	run(t, "export", "hosts", "--format", "ssh_config", "--out", newF)
	want, _ := os.ReadFile(oldF) //nolint:gosec // test temp path
	got, _ := os.ReadFile(newF)  //nolint:gosec // test temp path
	if len(want) == 0 || !bytes.Equal(got, want) {
		t.Errorf("ssh_config differs from config export --flat:\n--- new\n%s\n--- old\n%s", got, want)
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
