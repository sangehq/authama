package change

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/history"
)

// Moving the key root moves every key, so it is made only when the new name
// is typed again; anything else, or a name ssh or the manifest would refuse,
// fails the preview and moves nothing.
func TestKeyRootNeedsTheNameTypedAgain(t *testing.T) {
	p := home(t)
	key := filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519")
	for _, tc := range []struct{ target, confirm, want string }{
		{"keys", "", "type the new directory name again"},
		{"keys", "Keys", "type the new directory name again"},
		{"profiles", "profiles", "already ~/.ssh/profiles/"},
		{".hidden", ".hidden", "hidden"},
		{"a/b", "a/b", ""},
	} {
		plan, err := Preview(p, KeyRoot(tc.target, tc.confirm))
		if err != nil {
			t.Fatal(err)
		}
		if plan.Err == "" || !strings.Contains(plan.Err, tc.want) {
			t.Errorf("%q confirmed %q: preview error %q, want one saying %q", tc.target, tc.confirm, plan.Err, tc.want)
		}
		if _, _, err := Apply(p, KeyRoot(tc.target, tc.confirm), plan.Hash, 10, history.SourceWeb); err == nil {
			t.Errorf("%q confirmed %q was applied", tc.target, tc.confirm)
		}
	}
	if _, err := os.Stat(key); err != nil {
		t.Fatalf("a refused move moved the key: %v", err)
	}
}

// The preview lists every key the move takes, from a copy holding none of
// them; apply moves the real keys and renders the config; moving back to
// profiles puts every key where it was.
func TestKeyRootMovesAndMovesBack(t *testing.T) {
	p := home(t)
	from := filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519")
	to := filepath.Join(p.SSHDir, "keys", "work", "work_gh-ed25519")
	before, _ := os.ReadFile(from)

	ch := KeyRoot("keys", "keys")
	plan, err := Preview(p, ch)
	if err != nil || plan.Err != "" {
		t.Fatalf("preview: %v %s", err, plan.Err)
	}
	hidden := strings.Join(plan.Hidden, "\n")
	for _, want := range []string{"removes ~/.ssh/profiles/work/work_gh-ed25519", "creates ~/.ssh/keys/work/work_gh-ed25519"} {
		if !strings.Contains(hidden, want) {
			t.Errorf("the preview does not say %q:\n%s", want, hidden)
		}
	}
	if _, err := os.Stat(from); err != nil {
		t.Fatal("the preview moved the real key")
	}
	if _, _, err := Apply(p, ch, plan.Hash, 10, history.SourceWeb); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got, _ := os.ReadFile(to); string(got) != string(before) {
		t.Fatalf("the key is not at %s after the move", to)
	}
	if root := manifest.KeyRootAt(p.Manifest()); root != "keys" {
		t.Errorf("defaults.key_root is %q, want keys", root)
	}
	if cfg, _ := os.ReadFile(filepath.Join(p.SSHDir, "config")); !strings.Contains(string(cfg), "keys/work/work_gh-ed25519") {
		t.Errorf("~/.ssh/config was not rendered for the new root:\n%s", cfg)
	}

	back := KeyRoot("profiles", "profiles")
	plan, err = Preview(p, back)
	if err != nil || plan.Err != "" {
		t.Fatalf("preview back: %v %s", err, plan.Err)
	}
	if _, _, err := Apply(p, back, plan.Hash, 10, history.SourceWeb); err != nil {
		t.Fatalf("apply back: %v", err)
	}
	if got, _ := os.ReadFile(from); string(got) != string(before) {
		t.Error("moving back did not put the key where it was")
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "keys")); !os.IsNotExist(err) {
		t.Errorf("~/.ssh/keys is still there after moving back: %v", err)
	}
}
