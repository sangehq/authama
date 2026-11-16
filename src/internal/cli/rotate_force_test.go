package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/paths"
)

// forceHome is two keys whose targets take the key by hand (ploi), which a
// rotation with no terminal cannot get through, and a key no host uses.
func forceHome(t *testing.T) (paths.Paths, func() string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen not on PATH")
	}
	p := editHome(t)
	t.Setenv("AUTHAMA_AGE_RECIPIENT", "")
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "panel-a", "-H", "ploi-a.example", "-u", "x", "--provider", "ploi")
	run(t, "ssh", "host", "add", "work", "panel-b", "-H", "ploi-b.example", "-u", "x", "--provider", "ploi")
	run(t, "ssh", "key", "add", "work", "work_spare-ed25519")
	run(t, "ssh", "reconcile", "--yes")
	pubs := func() string {
		var b strings.Builder
		for _, n := range []string{"work_panel-a-ed25519", "work_panel-b-ed25519", "work_spare-ed25519"} {
			got, _ := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", n+".pub"))
			b.Write(got)
		}
		return b.String()
	}
	return p, pubs
}

// --force is a factory reset of the keys chosen: every one ends replaced,
// the paste targets without stopping to ask and the key no host uses too,
// and the run passes while saying what is left to deploy.
func TestAForcedRotationReplacesEveryKeyChosen(t *testing.T) {
	p, pubs := forceHome(t)
	before := pubs()
	out := run(t, "ssh", "rotate", "--all", "--force", "--yes", "--no-key-backup")
	after := pubs()
	for _, n := range []string{"work_panel-a-ed25519", "work_panel-b-ed25519", "work_spare-ed25519"} {
		got, _ := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", n+".pub"))
		if len(got) == 0 || strings.Contains(before, string(got)) {
			t.Errorf("%s was not replaced", n)
		}
	}
	if after == before {
		t.Fatal("nothing was replaced")
	}
	for _, want := range []string{"3 replaced", "Needs you", "deploy next"} {
		if !strings.Contains(out, want) {
			t.Errorf("the forced run does not say %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no host uses it, so there is nothing to rotate it on\n") && strings.Contains(out, "skip ") {
		t.Errorf("the forced run skipped the key no host uses:\n%s", out)
	}
}

// --check --force says what the forced run would do, changes nothing, and
// passes when every key would be rotated or replaced.
func TestAForcedDryRunPassesWhenEveryKeyWouldBeReplaced(t *testing.T) {
	_, pubs := forceHome(t)
	before := pubs()
	out := run(t, "ssh", "rotate", "--all", "--check", "--force")
	if pubs() != before {
		t.Fatal("a dry run changed a key")
	}
	for _, want := range []string{"3 would replace here", "--force", "no host uses it", "takes the new key by hand"} {
		if !strings.Contains(out, want) {
			t.Errorf("the forced dry run does not say %q:\n%s", want, out)
		}
	}
}

// The confirmation says it is a forced replace, and of how many keys.
func TestAForcedRotationAsksForAForcedReplace(t *testing.T) {
	_, pubs := forceHome(t)
	before := pubs()
	out, err := runIn(t, "n\n", "ssh", "rotate", "--all", "--force", "--no-key-backup")
	if err == nil {
		t.Fatal("answering no did not abort")
	}
	if !strings.Contains(out, "forced replace of 3 key(s)") {
		t.Errorf("the prompt does not say it is a forced replace of 3 keys:\n%s", out)
	}
	if pubs() != before {
		t.Error("an aborted forced rotation changed a key")
	}
}
