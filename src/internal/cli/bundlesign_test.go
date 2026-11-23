package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/bundlesig"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// setup export --sign-key signs the file and trusts the key in this home;
// another home refuses it with --require-signature until it trusts the key,
// and every home refuses it once it is changed.
func TestASignedExportIsCheckedOnImport(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	dir := t.TempDir()
	key := filepath.Join(dir, "signer")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "me@example.test", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	_ = perms.SetPerms(key, 0o600)
	doc := filepath.Join(dir, "setup.json")
	out := run(t, "setup", "export", "--out", doc, "--sign-key", key)
	if !strings.Contains(out, "signed: "+doc+".sig") || !strings.Contains(out, `namespaces="authama-bundle"`) {
		t.Fatalf("export:\n%s", out)
	}
	signers := bundlesig.SignersPath(p)
	if b, err := os.ReadFile(signers); err != nil || !strings.Contains(string(b), "me@example.test") {
		t.Fatalf("bundle_signers: %v\n%s", err, b)
	}
	if _, err := runErr(t, "setup", "export", "--sign-key", key); err == nil {
		t.Error("--sign-key without --out was taken")
	}

	editHome(t)
	if _, err := runErr(t, "setup", "import", doc, "--dry-run", "--require-signature"); err == nil {
		t.Fatal("a home that trusts no one took the export with --require-signature")
	}
	run(t, "setup", "import", doc, "--dry-run") // signed but untrusted: fine without --require-signature
	run(t, "setup", "import", doc, "--dry-run", "--require-signature", "--signers", signers)

	body, _ := os.ReadFile(doc)
	if err := os.WriteFile(doc, append(body, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr(t, "setup", "import", doc, "--dry-run"); err == nil || !strings.Contains(err.Error(), "changed after it was signed") {
		t.Fatalf("a changed export: %v", err)
	}
}
