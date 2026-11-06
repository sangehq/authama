package manifest

import (
	"path/filepath"
	"testing"
)

func TestCheckKeyRoot(t *testing.T) {
	for _, ok := range []string{"", "profiles", "keys", "id", "ssh-keys", "Work_Keys", "k2"} {
		if err := CheckKeyRoot(ok); err != nil {
			t.Errorf("CheckKeyRoot(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{".", "..", "a/b", `a\b`, ".keys", "-x", "with space", "config", "Config",
		"known_hosts", "known_hosts2", "authorized_keys", "authorized_keys2", "environment", "rc", "id_rsa", "k.pub"} {
		if err := CheckKeyRoot(bad); err == nil {
			t.Errorf("CheckKeyRoot(%q) = nil, want a refusal", bad)
		}
	}
}

// The key root is read in one place, and every path to a key is built from it.
func TestEveryKeyPathFollowsTheKeyRoot(t *testing.T) {
	m := Starter(false)
	m.SetProfile("work", Profile{KeyScope: "per_service"})
	m.SetProfile("acme", Profile{KeyScope: "per_service", Category: "clients"})
	if got := KeyRoot(m); got != DefaultKeyRoot {
		t.Fatalf("KeyRoot of a starter manifest = %q", got)
	}
	if got := KeyRoot(nil); got != DefaultKeyRoot {
		t.Fatalf("KeyRoot(nil) = %q", got)
	}
	m.Defaults.KeyRoot = "keys"
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := [][2]string{
		{m.ProfileRel("work"), "keys/work"},
		{m.ProfileRel("acme"), "keys/clients/acme"},
		{filepath.ToSlash(m.ProfileDir("/h/.ssh", "work")), "/h/.ssh/keys/work"},
		{CategoryConfigRelIn(KeyRoot(m), "clients"), "keys/clients/config"},
		{CategoryConfigFileIn(KeyRoot(m), "clients"), "~/.ssh/keys/clients/config"},
		{m.IdentityFile("work", "work_gh-ed25519"), "~/.ssh/keys/work/work_gh-ed25519"},
		{filepath.ToSlash(ArchiveDir(m, "/h/.ssh", KeyRef{Profile: "acme", KeyName: "k"})), "/h/.ssh/keys/clients/acme/old"},
	}
	for _, c := range cases {
		if c[0] != c[1] {
			t.Errorf("got %q, want %q", c[0], c[1])
		}
	}
	m.Defaults.KeyRoot = "a/b"
	if err := m.Validate(); err == nil {
		t.Error("a nested key root validated")
	}
}

// A reserved key is a file directly in ~/.ssh, so it cannot share the key
// root's name.
func TestAReservedKeyCannotTakeTheKeyRootsName(t *testing.T) {
	m := Starter(false)
	m.Defaults.KeyRoot = "keys"
	Reserve(m, "keys", ReservedKey{})
	if err := m.Validate(); err == nil {
		t.Fatal("a reserved key named like the key root validated")
	}
}
