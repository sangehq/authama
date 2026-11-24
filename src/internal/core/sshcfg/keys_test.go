package sshcfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPub = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB/22QXk5vTmOo31cReZdmvraArThvJ6FLu4g9t8PFIF test\n"

func writePair(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("private-stand-in"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".pub"), []byte(testPub), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWalkKeysFindsNestedPairsAndSkipsNoise(t *testing.T) {
	ssh := testSSHDir(t)
	writePair(t, ssh, "docker-tunnel")
	writePair(t, filepath.Join(ssh, "work", "prod"), "deploy")
	writePair(t, filepath.Join(ssh, "profiles", "personal", "me"), "id")
	// Noise that must be skipped.
	if err := os.Mkdir(filepath.Join(ssh, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssh, "agent", "s.socket"), []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(ssh, "authama-cm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssh, "known_hosts"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssh, "authorized_keys"), []byte(testPub), 0o644); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(ssh, "config"), "Host x\n")
	write(t, filepath.Join(ssh, "config.d", "work.conf"), "Host w\n")

	keys := WalkKeys(ssh)
	got := map[string]bool{}
	for _, k := range keys {
		got[k.Rel] = true
	}
	for _, want := range []string{"docker-tunnel", "work/prod/deploy", "profiles/personal/me/id"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	for name := range got {
		if strings.Contains(name, "agent") || strings.Contains(name, "known_hosts") ||
			strings.Contains(name, "authorized") || strings.Contains(name, "config") ||
			strings.Contains(name, "authama-cm") {
			t.Errorf("walked a skipped path: %s", name)
		}
	}

	// skipRoots omits the managed key tree.
	keys = WalkKeys(ssh, filepath.Join(ssh, "profiles"))
	got = map[string]bool{}
	for _, k := range keys {
		got[k.Rel] = true
	}
	if got["profiles/personal/me/id"] {
		t.Error("skipRoots did not skip the key root")
	}
	if !got["work/prod/deploy"] || !got["docker-tunnel"] {
		t.Errorf("skipRoots dropped keys it should have kept: %v", got)
	}
}

func TestWalkKeysFollowsSymlinksWithoutLooping(t *testing.T) {
	ssh := testSSHDir(t)
	real := filepath.Join(ssh, "real")
	writePair(t, real, "k")
	if err := os.Symlink(real, filepath.Join(ssh, "link")); err != nil {
		t.Fatal(err)
	}
	// A loop: loop/ -> ssh/loop (self) would hang without the seen set.
	loop := filepath.Join(ssh, "loopdir")
	if err := os.Mkdir(loop, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(loop, filepath.Join(loop, "self")); err != nil {
		t.Fatal(err)
	}
	keys := WalkKeys(ssh)
	var n int
	for _, k := range keys {
		if k.Rel == "real/k" || k.Rel == "link/k" {
			n++
		}
	}
	if n == 0 {
		t.Fatalf("symlink to a key dir was not followed: %+v", keys)
	}
	// Both the real path and the link may be reported once each, or
	// collapsed by dirID; either way the walk must return.
}
