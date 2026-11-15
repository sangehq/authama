package change

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/history"
)

// otherSSH is another machine's ~/.ssh: a config naming a key that lives
// beside it, outside any profile, which import adopts.
func otherSSH(t *testing.T) (dir string, key []byte) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "other-ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	key = make([]byte, 48)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	cfg := "Host legacy\n  HostName legacy.example\n  User ops\n  IdentityFile ~/.ssh/legacy_key\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy_key"), key, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, key
}

// Import adopts keys from outside the managed tree by copying them. Its
// preview shows each key it would create without copying one: the copy gets
// an empty file there, and apply copies the real key.
func TestImportPreviewCopiesNoPrivateKey(t *testing.T) {
	p := home(t)
	src, key := otherSSH(t)
	ch := SSHImport(p, src, true)

	// What the preview's copy is left holding.
	shadow, cleanup, err := copyHome(p)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var out bytes.Buffer
	if err := ch.Run(shadow, &out); err != nil {
		t.Fatalf("run in the copy: %v\n%s", err, out.String())
	}
	adopted, _ := filepath.Glob(filepath.Join(shadow.SSHDir, "profiles", "imported", "*legacy*"))
	if len(adopted) != 1 {
		t.Fatalf("the copy adopted %v, want one key", adopted)
	}
	if b, _ := os.ReadFile(adopted[0]); len(b) != 0 {
		t.Fatalf("the preview's copy holds %d bytes of the private key", len(b))
	}

	plan, err := Preview(p, ch)
	if err != nil || plan.Err != "" {
		t.Fatalf("preview: %v %s", err, plan.Err)
	}
	rel, _ := filepath.Rel(shadow.SSHDir, adopted[0])
	want := "creates ~/.ssh/" + filepath.ToSlash(rel)
	if !strings.Contains(strings.Join(plan.Hidden, "\n"), want) {
		t.Errorf("the preview does not say %q: %v", want, plan.Hidden)
	}
	if len(plan.Effects) != 1 || !strings.Contains(plan.Effects[0], "import-backup-") {
		t.Errorf("replacing a manifest says nothing of its backup: %v", plan.Effects)
	}

	if _, _, err := Apply(p, ch, plan.Hash, 10, history.SourceWeb); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(p.SSHDir, rel)); !bytes.Equal(got, key) {
		t.Error("apply did not adopt the key itself")
	}
	if backups, _ := filepath.Glob(filepath.Join(p.StateDir(), "import-backup-*", "manifest.json")); len(backups) != 1 {
		t.Errorf("apply took %d backups of the manifest it replaced, want 1", len(backups))
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil || len(m.Profiles) != 1 {
		t.Fatalf("the manifest after import: %v %v", err, m)
	}
}

// Import replaces the manifest, so one with profiles is refused unless the
// person said to replace it, as the CLI refuses it without --force.
func TestImportWillNotReplaceAManifestUnasked(t *testing.T) {
	p := home(t)
	src, _ := otherSSH(t)
	before, _ := os.ReadFile(p.Manifest())
	plan, err := Preview(p, SSHImport(p, src, false))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Err != ErrImportWouldReplace.Error() {
		t.Fatalf("preview error %q, want the CLI's refusal", plan.Err)
	}
	if _, _, err := Apply(p, SSHImport(p, src, false), plan.Hash, 10, history.SourceWeb); err == nil ||
		!strings.Contains(err.Error(), "non-empty manifest") {
		t.Fatalf("apply: %v", err)
	}
	if after, _ := os.ReadFile(p.Manifest()); !bytes.Equal(before, after) {
		t.Error("a refused import changed the manifest")
	}
}
