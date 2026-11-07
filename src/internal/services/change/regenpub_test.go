package change

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keysvc"
)

// The preview's copy holds an empty stand-in for each private key, so a .pub
// derived there would be derived from nothing. regen-pub reads the real
// private key instead: the preview shows the .pub it would really write, and
// apply writes that same one.
func TestRegenPubPreviewsTheRealPublicKey(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not installed")
	}
	p := home(t)
	priv := filepath.Join(p.SSHDir, "profiles", "work", "work_gh-ed25519")
	_ = os.Remove(priv)
	_ = os.Remove(priv + ".pub")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "work gh", "-f", priv).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, out)
	}
	want, _ := os.ReadFile(priv + ".pub")
	blob := strings.Fields(string(want))[1]
	// A .pub that is not this key's: the case regen-pub repairs.
	if err := os.WriteFile(priv+".pub", []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl work gh\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ch := KeyRegenPub(p, keysvc.Selection{Selectors: []string{"work/work_gh-ed25519"}})
	plan, err := Preview(p, ch)
	if err != nil || plan.Err != "" {
		t.Fatalf("preview: %v %s", err, plan.Err)
	}
	if !strings.Contains(plan.Output, "rewritten work/work_gh-ed25519") || !strings.Contains(plan.Output, " 0 failed") {
		t.Errorf("the preview derived from the stand-in, not the key:\n%s", plan.Output)
	}
	shown := false
	for _, f := range plan.Files {
		if strings.HasSuffix(f.Path, "work_gh-ed25519.pub") {
			for _, l := range f.Lines {
				shown = shown || strings.Contains(l.Text, blob)
			}
		}
	}
	if !shown {
		t.Errorf("the preview does not show the real public key:\n%+v\n%s", plan.Files, plan.Output)
	}
	if got, _ := os.ReadFile(priv + ".pub"); strings.Contains(string(got), blob) {
		t.Fatal("the preview wrote the real .pub")
	}

	if _, _, err := Apply(p, ch, plan.Hash, 10, history.SourceWeb); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.ReadFile(priv + ".pub")
	if !strings.Contains(string(got), blob) {
		t.Errorf("apply wrote %q, want the key's own public key", got)
	}
}
