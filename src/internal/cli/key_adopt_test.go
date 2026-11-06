package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A key the user keeps at ~/.ssh/id_ed25519 is adopted where it is: the
// rendered config points at that path, nothing calls it loose, a forced
// regenerate keeps it there and archives the old pair in the profile's old/,
// and the file is never moved.
func TestAnAdoptedKeyStaysWhereTheUserKeepsIt(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen")
	}
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	priv := filepath.Join(p.SSHDir, "id_ed25519")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "mine", "-f", priv).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, out)
	}
	run(t, "ssh", "profile", "add", "personal")
	run(t, "ssh", "host", "add", "personal", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "key", "adopt", "personal/github", "~/.ssh/id_ed25519", "--host", "gh", "--yes")

	run(t, "ssh", "config", "render", "--yes")
	cfg, _ := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if !strings.Contains(string(cfg), "IdentityFile ~/.ssh/id_ed25519") {
		t.Fatalf("the rendered config does not point at the adopted key:\n%s", cfg)
	}
	if out, _ := runErr(t, "doctor"); strings.Contains(out, "loose") && strings.Contains(out, "id_ed25519") {
		t.Fatalf("doctor calls the adopted key loose:\n%s", out)
	}

	before, _ := os.ReadFile(priv + ".pub")
	run(t, "ssh", "keygen", "personal/github", "--force", "--yes", "--no-key-backup")
	after, err := os.ReadFile(priv + ".pub")
	if err != nil || string(after) == string(before) {
		t.Fatalf("no new key at the adopted path: %v", err)
	}
	archived, err := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "personal", "old", "github.pub"))
	if err != nil || string(archived) != string(before) {
		t.Fatalf("the replaced pair is not in the profile's old/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "old")); err == nil {
		t.Fatal("an old/ appeared directly in ~/.ssh")
	}

	// Adopting a public key, a path out of ~/.ssh, or the same file twice is refused.
	for _, bad := range [][]string{
		{"personal/pub", "id_ed25519.pub"},
		{"personal/out", "../etc/passwd"},
		{"personal/twice", "id_ed25519"},
	} {
		if _, err := runErr(t, append([]string{"ssh", "key", "adopt"}, append(bad, "--yes")...)...); err == nil {
			t.Errorf("adopt %v was accepted", bad)
		}
	}
}
