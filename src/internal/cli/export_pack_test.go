package cli

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// `export pack` zips the parts chosen; `setup import` takes the zip for its
// setup.json, and refuses one it cannot import with the reason.
func TestExportPackAndImportIt(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "web", "-H", "web.example.test", "-u", "deploy")
	if out := run(t, "export", "pack", "--include", "all", "--dry-run"); !strings.Contains(out, "termius-hosts.csv") ||
		!strings.Contains(out, "structure.json") || !strings.Contains(out, "nothing written") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := runErr(t, "export", "pack", "--include", "nope"); err == nil {
		t.Error("an unknown part was accepted")
	}
	pack := filepath.Join(t.TempDir(), "pack.zip")
	run(t, "export", "pack", "--include", "setup,termius,ssh-config", "--termius-groups", "category/profile", "--out", pack)
	zr, err := zip.OpenReader(pack)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	_ = zr.Close()
	if strings.Join(names, " ") != "README.txt termius-hosts.csv ssh_config setup.json" {
		t.Errorf("pack members: %v", names)
	}

	q := editHome(t)
	if out := run(t, "setup", "import", pack, "--dry-run"); !strings.Contains(out, "profile      work") {
		t.Fatalf("import dry run:\n%s", out)
	}
	run(t, "setup", "import", pack, "-y")
	if m, err := manifest.Load(q.Manifest()); err != nil || len(m.Profiles["work"].Hosts) != 1 {
		t.Fatalf("imported: %v", err)
	}
	only := filepath.Join(t.TempDir(), "hosts.zip")
	run(t, "export", "pack", "--include", "termius", "--out", only)
	if _, err := runErr(t, "setup", "import", only); err == nil || !strings.Contains(err.Error(), "no setup.json") {
		t.Errorf("a pack without setup.json: %v", err)
	}
	cfg := filepath.Join(t.TempDir(), "config")
	_ = os.WriteFile(cfg, []byte("Host x\n  HostName x.example\n"), 0o600)
	if _, err := runErr(t, "setup", "import", cfg); err == nil || !strings.Contains(err.Error(), "ssh import") {
		t.Errorf("an ssh config: %v", err)
	}
}

// The ssh-clients part writes each other client's hosts under clients/, with
// notes on finishing each import, and no private key.
func TestExportPackHoldsEveryClientFormat(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "web", "-H", "web.example.test", "-u", "deploy")
	run(t, "ssh", "reconcile", "-y")
	pack := filepath.Join(t.TempDir(), "clients.zip")
	run(t, "export", "pack", "--include", "ssh-clients", "--out", pack)
	zr, err := zip.OpenReader(pack)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	seen := map[string]bool{}
	for _, f := range zr.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) == 3 && parts[0] == "clients" {
			seen[parts[1]] = true
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		if strings.Contains(string(body), "PRIVATE KEY") {
			t.Errorf("%s holds a private key", f.Name)
		}
	}
	for _, name := range []string{"hosts-csv", "windows-terminal", "iterm2", "putty", "remmina", "securecrt", "mobaxterm"} {
		if !seen[name] {
			t.Errorf("no clients/%s/ in the pack", name)
		}
	}
	if _, err := zr.Open("clients/NOTES.txt"); err != nil {
		t.Error("no clients/NOTES.txt")
	}
}
