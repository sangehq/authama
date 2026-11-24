package structure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// IdentityFile lines in a file ~/.ssh/config Includes are discovered, but
// a file authama does not manage is never rewritten: the owner is told to
// update it by hand.
func TestRefsFollowIncludesAndDoNotRewriteUnmanagedFiles(t *testing.T) {
	p := home(t, "")
	pair(t, filepath.Join(p.SSHDir, "id_work"), pubWork)
	outside := filepath.Join(t.TempDir(), "orb", "ssh")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(outside, "config")
	if err := os.WriteFile(foreign, []byte("Host orb\n  IdentityFile ~/.ssh/id_work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(p.SSHDir, "config"),
		"Include "+foreign+"\nHost x\n  IdentityFile ~/.ssh/id_work\n", 0o600)

	refs := ConfigRefs(p)
	key := filepath.Join(p.SSHDir, "id_work")
	if len(refs[key]) < 2 {
		t.Fatalf("ConfigRefs should name both the root config and the foreign include, got %v", refs[key])
	}
	manual := ManualRefs(p, key)
	if len(manual) == 0 {
		t.Fatal("ManualRefs should list the foreign include")
	}
	changed, err := rewriteRefs(p, key, filepath.Join(p.SSHDir, "profiles", "work", "id_work"))
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || !strings.Contains(changed[0], "config") {
		t.Fatalf("should have rewritten only the managed root config, got %v", changed)
	}
	root, _ := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if !strings.Contains(string(root), "profiles/work/id_work") {
		t.Errorf("managed config not rewritten:\n%s", root)
	}
	foreignBody, _ := os.ReadFile(foreign)
	if strings.Contains(string(foreignBody), "profiles/work") {
		t.Errorf("unmanaged include was rewritten:\n%s", foreignBody)
	}
	if !strings.Contains(string(foreignBody), "~/.ssh/id_work") {
		t.Errorf("unmanaged include lost its original IdentityFile:\n%s", foreignBody)
	}
}
