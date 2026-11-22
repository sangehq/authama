package cli

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

func TestSetupExportImportDocument(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "web", "-H", "web.example.test", "-u", "deploy")
	doc := filepath.Join(t.TempDir(), "setup.json")
	out := run(t, "setup", "export", "--out", doc)
	if !strings.Contains(out, "1 profile(s)") {
		t.Fatalf("export:\n%s", out)
	}
	if fi, err := os.Stat(doc); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("document: %v %v", err, fi)
	}

	q := editHome(t)
	starter, _ := os.ReadFile(q.Manifest())
	out = run(t, "setup", "import", doc, "--dry-run")
	if !strings.Contains(out, "add      profile      work") || !strings.Contains(out, "config/manifest.json") {
		t.Fatalf("dry run:\n%s", out)
	}
	if now, _ := os.ReadFile(q.Manifest()); string(now) != string(starter) {
		t.Fatal("a dry run wrote the manifest")
	}
	run(t, "setup", "import", doc, "-y")
	m, err := manifest.Load(q.Manifest())
	if err != nil || len(m.Profiles["work"].Hosts) != 1 {
		t.Fatalf("imported manifest: %v %+v", err, m)
	}
	if cfg, _ := os.ReadFile(filepath.Join(q.SSHDir, "config")); !strings.Contains(string(cfg), "HostName web.example.test") {
		t.Errorf("the import did not render ~/.ssh/config:\n%s", cfg)
	}
	if out := run(t, "history", "--command", "setup import"); !strings.Contains(out, "setup import") {
		t.Errorf("history:\n%s", out)
	}
	if out := run(t, "setup", "import", doc, "--on-conflict", "fail", "-y"); !strings.Contains(out, "nothing to change") {
		t.Errorf("a second import:\n%s", out)
	}
	if _, err := runErr(t, "setup", "import", doc, "--on-conflict", "maybe"); err == nil {
		t.Error("an unknown policy was accepted")
	}
}

func TestSetupEncryptedExportCarriesKeysAndTokens(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "web", "-H", "web.example.test", "-u", "deploy")
	run(t, "ssh", "reconcile", "--yes")
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	refs, err := m.KeyRefs()
	if err != nil || len(refs) == 0 {
		t.Fatalf("keys: %v %v", refs, err)
	}
	ref := refs[0]
	priv, err := os.ReadFile(m.KeyPath(p.SSHDir, ref))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Tokens(), []byte(`{"composer":{"github.com":{"type":"github-oauth","env":"SETUP_TEST_TOKEN"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	value := hex.EncodeToString(b)
	t.Setenv("SETUP_TEST_TOKEN", value)
	dir := t.TempDir()
	passFile := filepath.Join(dir, "pass")
	_, _ = rand.Read(b)
	if err := os.WriteFile(passFile, []byte(hex.EncodeToString(b)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr(t, "setup", "export", "--encrypt"); err == nil {
		t.Fatal("--encrypt without --out was accepted")
	}
	age := filepath.Join(dir, "setup.age")
	out := run(t, "setup", "export", "--encrypt", "--out", age, "--passphrase-file", passFile)
	if !strings.Contains(out, "wrote "+age) {
		t.Fatalf("export:\n%s", out)
	}
	raw, _ := os.ReadFile(age)
	if strings.Contains(string(raw), value) || strings.Contains(string(raw), string(priv)) {
		t.Fatal("the export holds plaintext")
	}
	if out, err := runErr(t, "ssh", "restore", age, "-y"); err == nil || !strings.Contains(err.Error(), "setup import") {
		t.Fatalf("restore of a setup export: %v\n%s", err, out)
	}
	loose := filepath.Join(dir, "loose")
	if err := os.WriteFile(loose, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr(t, "setup", "export", "--encrypt", "--out", filepath.Join(dir, "b.age"), "--passphrase-file", loose); err == nil {
		t.Fatal("a world-readable passphrase file was accepted")
	}

	q := editHome(t)
	t.Setenv("SETUP_TEST_TOKEN", "")
	_ = os.Unsetenv("SETUP_TEST_TOKEN")
	out = run(t, "setup", "import", age, "--passphrase-file", passFile, "--dry-run")
	if !strings.Contains(out, "SETUP_TEST_TOKEN") || strings.Contains(out, value) || strings.Contains(out, "PRIVATE KEY") {
		t.Fatalf("dry run shows names, never values:\n%s", out)
	}
	run(t, "setup", "import", age, "--passphrase-file", passFile, "-y")
	qm, err := manifest.Load(q.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(qm.KeyPath(q.SSHDir, ref))
	if err != nil || string(got) != string(priv) {
		t.Fatalf("the key was not imported: %v", err)
	}
	env, _ := os.ReadFile(q.EnvFile())
	if !strings.Contains(string(env), `SETUP_TEST_TOKEN="`+value+`"`) {
		t.Fatalf(".env:\n%s", env)
	}
	if fi, _ := os.Stat(q.EnvFile()); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf(".env mode %v", fi.Mode())
	}
}
