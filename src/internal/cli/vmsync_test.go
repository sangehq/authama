package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A VM host whose VM the hypervisor no longer lists: --dry-run shows the
// removal and writes nothing; the real run backs the manifest up and removes
// it; a second run has nothing to do.
func TestVMSyncDryRunThenPrunesAGoneVMOnce(t *testing.T) {
	p := editHome(t)
	run(t, "ssh", "profile", "add", "devbox")
	run(t, "ssh", "vm", "add", "devbox", "old", "--hypervisor", "lima", "--vm", "gone", "-u", "me")
	bin := t.TempDir()
	stubBin(t, bin, "limactl", `[ "$1" = list ] && printf '%s\n' '{"name":"default","status":"Stopped"}'`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("AUTHAMA_VM_HYPERVISORS", "lima")
	before, _ := os.ReadFile(p.Manifest())
	out := run(t, "ssh", "vm", "sync", "--dry-run")
	if !strings.Contains(out, "remove host devbox/old") || !strings.Contains(out, "default") && !strings.Contains(out, "gone") {
		t.Errorf("dry run:\n%s", out)
	}
	if after, _ := os.ReadFile(p.Manifest()); string(after) != string(before) {
		t.Fatal("--dry-run changed the manifest")
	}
	run(t, "ssh", "vm", "sync", "--yes")
	if after, _ := os.ReadFile(p.Manifest()); strings.Contains(string(after), `"old"`) {
		t.Errorf("the gone host is still there:\n%s", after)
	}
	backups, _ := filepath.Glob(filepath.Join(p.State(), "backups", "vmsync", "*"))
	if len(backups) == 0 {
		t.Error("no manifest backup was taken")
	}
	if out := run(t, "ssh", "vm", "sync", "--yes", "--quiet"); strings.TrimSpace(out) != "" {
		t.Errorf("a quiet sync with nothing to do printed:\n%s", out)
	}
}

func TestVMAdoptKeyCopiesALooseKeyAndRefusesBadTargets(t *testing.T) {
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(p.SSHDir, "box-tunnel")
	if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "docker tunnel", "-f", key).Run(); err != nil {
		t.Skip("ssh-keygen: ", err)
	}
	if _, err := runErr(t, "ssh", "vm", "adopt-key", "box-tunnel", "devbox", "box", "--hypervisor", "orbstack", "--vm", "u", "-u", "me", "--yes"); err == nil {
		t.Error("adopted onto an own-key hypervisor")
	}
	if _, err := runErr(t, "ssh", "vm", "adopt-key", "box-tunnel", "devbox", "box", "--hypervisor", "parallels", "--vm", "u", "-u", "me;rm", "--yes"); err == nil {
		t.Error("accepted a bad guest user")
	}
	out := run(t, "ssh", "vm", "adopt-key", "box-tunnel", "devbox", "box", "--hypervisor", "parallels", "--vm", "Ubuntu", "-u", "me", "--dry-run")
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "devbox")); err == nil {
		t.Errorf("--dry-run wrote the profile:\n%s", out)
	}
	run(t, "ssh", "vm", "adopt-key", "box-tunnel", "devbox", "box", "--hypervisor", "parallels", "--vm", "Ubuntu", "-u", "me", "--yes")
	for _, f := range []string{key, key + ".pub"} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("the loose file %s was not kept: %v", f, err)
		}
	}
	m, _ := os.ReadFile(p.Manifest())
	if !strings.Contains(string(m), `"box"`) || !strings.Contains(string(m), `"parallels"`) {
		t.Errorf("manifest:\n%s", m)
	}
	if _, err := runErr(t, "ssh", "vm", "adopt-key", "box-tunnel", "devbox", "box", "--hypervisor", "parallels", "--vm", "Ubuntu", "-u", "me", "--yes"); err == nil {
		t.Error("adopted twice onto the same alias")
	}
}
