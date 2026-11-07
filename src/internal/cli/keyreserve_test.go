package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Reserving a loose key moves it out of doctor's loose findings into an info
// block, shows in key list and show, never touches the file, and adopting the
// key onto a VM releases the reservation.
func TestKeyReserveLifecycle(t *testing.T) {
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(p.SSHDir, "docker-tunnel")
	if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "mac-to-devbox-docker-tunnel", "-f", key).Run(); err != nil {
		t.Skip("ssh-keygen: ", err)
	}
	before, _ := os.ReadFile(key)
	run(t, "ssh", "config", "render", "--yes")
	if out, _ := runErr(t, "doctor"); !strings.Contains(out, "loose:") {
		t.Fatalf("the key should start loose:\n%s", out)
	}
	if _, err := runErr(t, "ssh", "key", "reserve", filepath.Join(t.TempDir(), "x"), "--yes"); err == nil {
		t.Error("reserved a file outside ~/.ssh")
	}
	if _, err := runErr(t, "ssh", "key", "reserve", "nope", "--yes"); err == nil {
		t.Error("reserved a file that does not exist")
	}
	manifestBefore, _ := os.ReadFile(p.Manifest())
	run(t, "ssh", "key", "reserve", "docker-tunnel", "--for", "devbox", "--note", "Docker VM, later", "--dry-run")
	if after, _ := os.ReadFile(p.Manifest()); string(after) != string(manifestBefore) {
		t.Fatal("--dry-run changed the manifest")
	}
	run(t, "ssh", "key", "reserve", key+".pub", "--for", "devbox", "--note", "Docker VM, later", "--yes")
	out, err := runErr(t, "doctor", "--strict")
	if err != nil || strings.Contains(out, "loose:") ||
		!strings.Contains(out, "docker-tunnel - reserved for devbox: Docker VM, later") {
		t.Fatalf("doctor after reserving (%v):\n%s", err, out)
	}
	if js := run(t, "doctor", "--json"); !strings.Contains(js, `"reserved_keys": [`) || !strings.Contains(js, `"state": "kept"`) {
		t.Errorf("doctor --json:\n%s", js)
	}
	if out := run(t, "ssh", "key", "list"); !strings.Contains(out, "docker-tunnel - reserved for devbox") {
		t.Errorf("key list:\n%s", out)
	}
	if out := run(t, "ssh", "show", "docker-tunnel"); !strings.Contains(out, "reserved key docker-tunnel") || !strings.Contains(out, "SHA256:") {
		t.Errorf("show:\n%s", out)
	}
	run(t, "ssh", "vm", "adopt-key", "docker-tunnel", "devbox", "docker", "--hypervisor", "parallels", "--vm", "Ubuntu", "-u", "me", "--yes")
	if m, _ := os.ReadFile(p.Manifest()); strings.Contains(string(m), "reserved_keys") {
		t.Errorf("adopting should release the reservation:\n%s", m)
	}
	if after, _ := os.ReadFile(key); string(after) != string(before) {
		t.Error("the key file changed")
	}
	if _, err := runErr(t, "ssh", "key", "unreserve", "docker-tunnel", "--yes"); err == nil {
		t.Error("unreserved a key that is not reserved")
	}
}

func TestKeyUnreserve(t *testing.T) {
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(p.SSHDir, "spare-laptop")
	if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "old laptop", "-f", key).Run(); err != nil {
		t.Skip("ssh-keygen: ", err)
	}
	run(t, "ssh", "key", "reserve", "spare-laptop", "--yes")
	run(t, "ssh", "key", "unreserve", "spare-laptop", "--yes")
	if out, _ := runErr(t, "doctor"); !strings.Contains(out, "loose:") || strings.Contains(out, "reserved keys") {
		t.Errorf("after unreserve the key is loose again:\n%s", out)
	}
}

// A devbox declared in authama.json without a vm keeps its key reserved, and
// sync adds no host for it.
func TestPlannedDevboxKeepsItsKey(t *testing.T) {
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(p.SSHDir, "docker-tunnel")
	if err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "docker tunnel", "-f", key).Run(); err != nil {
		t.Skip("ssh-keygen: ", err)
	}
	if err := os.WriteFile(p.SettingsFile(), []byte(`{"version": 1, "devboxes": {"docker": {"profile": "devbox", "key": "docker-tunnel"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, "ssh", "config", "render", "--yes")
	out, err := runErr(t, "doctor", "--strict")
	if err != nil || !strings.Contains(out, "docker-tunnel - reserved for devbox docker") || strings.Contains(out, "loose:") {
		t.Fatalf("doctor (%v):\n%s", err, out)
	}
	before, _ := os.ReadFile(p.Manifest())
	if out := run(t, "ssh", "vm", "sync", "--yes"); !strings.Contains(out, "has no vm yet") {
		t.Errorf("sync:\n%s", out)
	}
	if after, _ := os.ReadFile(p.Manifest()); string(after) != string(before) {
		t.Error("sync changed the manifest for a planned devbox")
	}
}
