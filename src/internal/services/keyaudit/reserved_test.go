package keyaudit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

const tunnelPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEB mac-to-devbox-docker-tunnel\n"

func writeSSH(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A reserved key is information: it leaves the loose findings, passes even
// under --strict, and is never touched.
func TestReservedKeyIsNotLoose(t *testing.T) {
	f := newFixtureFrom(t, hostOnlyManifestJSON)
	writeSSH(t, f.ssh, "docker-tunnel", "PRIVATE\n")
	writeSSH(t, f.ssh, "docker-tunnel.pub", tunnelPub)
	if rep := audit(t, f, true); rep.OK() || len(rep.ByState(Loose)) != 1 {
		t.Fatalf("before reserving, the key should be loose and fail --strict: %+v", rep.Findings)
	}
	fp, err := FingerprintOf(tunnelPub)
	if err != nil || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fingerprint %q %v", fp, err)
	}
	manifest.Reserve(f.svc.m, "docker-tunnel", manifest.ReservedKey{For: "devbox", Note: "Docker VM", Fingerprint: fp})
	rep := audit(t, f, true)
	if !rep.OK() || len(rep.ByState(Loose)) != 0 {
		t.Fatalf("a reserved key should not be loose: %+v", rep.Findings)
	}
	if len(rep.Reserved) != 1 || rep.Reserved[0].State != ReservedKept || rep.Reserved[0].Source != "manifest" {
		t.Fatalf("reserved = %+v", rep.Reserved)
	}
	if l := strings.Join(rep.ReservedLines(), "\n"); !strings.Contains(l, "docker-tunnel - reserved for devbox: Docker VM") {
		t.Errorf("lines:\n%s", l)
	}
	if b, _ := os.ReadFile(filepath.Join(f.ssh, "docker-tunnel")); string(b) != "PRIVATE\n" {
		t.Error("the audit touched the key")
	}
}

func TestReservedKeyProblems(t *testing.T) {
	f := newFixtureFrom(t, hostOnlyManifestJSON)
	writeSSH(t, f.ssh, "swapped", "PRIVATE\n")
	writeSSH(t, f.ssh, "swapped.pub", tunnelPub)
	manifest.Reserve(f.svc.m, "swapped", manifest.ReservedKey{Fingerprint: "SHA256:other"})
	manifest.Reserve(f.svc.m, "gone", manifest.ReservedKey{For: "devbox"})
	got := map[string]Reserved{}
	rep := audit(t, f, false)
	for _, r := range rep.Reserved {
		got[r.Name] = r
	}
	if got["swapped"].State != ReservedChanged || got["gone"].State != ReservedMissing {
		t.Fatalf("states: %+v", rep.Reserved)
	}
	if !strings.Contains(got["gone"].Fix, "key unreserve gone") || len(rep.ReservedProblems()) != 2 {
		t.Errorf("fix %q, problems %d", got["gone"].Fix, len(rep.ReservedProblems()))
	}
}

// A devbox declared in authama.json without a VM keeps its key the same way.
func TestPlannedDevboxKeyIsReserved(t *testing.T) {
	f := newFixtureFrom(t, hostOnlyManifestJSON)
	writeSSH(t, f.ssh, "docker-tunnel", "PRIVATE\n")
	writeSSH(t, f.ssh, "docker-tunnel.pub", tunnelPub)
	f.svc.WithPlanned(map[string]manifest.ReservedKey{"docker-tunnel": {For: "devbox docker"}})
	rep := audit(t, f, true)
	if !rep.OK() || len(rep.Reserved) != 1 || rep.Reserved[0].Source != "devbox docker" {
		t.Fatalf("planned: %+v %+v", rep.Findings, rep.Reserved)
	}
	keys := MarkReserved(LooseKeysIn(f.ssh, manifest.DefaultKeyRoot), map[string]manifest.ReservedKey{"docker-tunnel": {For: "devbox"}})
	if len(keys) != 1 || !keys[0].Reserved || keys[0].ReservedFor != "devbox" {
		t.Errorf("marked: %+v", keys)
	}
}
