package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/structure"
)

func TestStructureSuggestsThenMovesOnlyWhenApproved(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	keygen(t, filepath.Join(p.SSHDir, "id_ed25519_work"), "me@work")
	keygen(t, filepath.Join(p.SSHDir, "id_spare"), "spare")

	var ss []structure.Suggestion
	if err := json.Unmarshal(jsonPart(run(t, "ssh", "structure", "list", "--json")), &ss); err != nil {
		t.Fatal(err)
	}
	if len(ss) != 2 {
		t.Fatalf("suggestions: %+v", ss)
	}
	// Listing and checking never move anything.
	run(t, "ssh", "structure", "check", "--quiet")
	if _, err := os.Stat(filepath.Join(p.SSHDir, "id_ed25519_work")); err != nil {
		t.Fatal("a check moved a key")
	}
	var work, spare structure.Suggestion
	for _, s := range ss {
		if strings.HasSuffix(s.Path, "work") {
			work = s
		} else {
			spare = s
		}
	}
	out := run(t, "ssh", "structure", "approve", work.ID, "--dry-run")
	if !strings.Contains(out, "creates ~/.ssh/profiles/work/id_ed25519_work") {
		t.Fatalf("dry run:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "id_ed25519_work")); err != nil {
		t.Fatal("a dry run moved a key")
	}
	if _, err := runErr(t, "ssh", "structure", "approve", spare.ID, "-y"); err == nil {
		t.Fatal("a key with no guessed profile moved without --profile")
	}
	run(t, "ssh", "structure", "approve", work.ID, "-y")
	run(t, "ssh", "structure", "approve", spare.ID[:5], "--profile", "work", "--name", "spare", "-y")
	for _, f := range []string{"profiles/work/id_ed25519_work", "profiles/work/spare.pub"} {
		if _, err := os.Stat(filepath.Join(p.SSHDir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if out := run(t, "ssh", "structure", "list"); !strings.Contains(out, "everything is where") {
		t.Errorf("after:\n%s", out)
	}
	if out := run(t, "history", "--command", "structure approve"); !strings.Contains(out, "structure approve") {
		t.Errorf("history:\n%s", out)
	}
}

func TestStructureDismissAndPlanFile(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	_ = os.MkdirAll(p.SSHDir, 0o700)
	keygen(t, filepath.Join(p.SSHDir, "id_x"), "x")
	var ss []structure.Suggestion
	_ = json.Unmarshal(jsonPart(run(t, "ssh", "structure", "list", "--json")), &ss)
	if len(ss) != 1 {
		t.Fatalf("%+v", ss)
	}
	run(t, "ssh", "structure", "dismiss", ss[0].ID)
	if out := run(t, "ssh", "structure", "list"); !strings.Contains(out, "everything is where") {
		t.Errorf("dismissed still listed:\n%s", out)
	}
	if out := run(t, "ssh", "structure", "list", "--all"); !strings.Contains(out, "(dismissed)") {
		t.Errorf("--all:\n%s", out)
	}
	plan := filepath.Join(t.TempDir(), "plan.txt")
	_ = os.WriteFile(plan, []byte("key ~/.ssh/id_x profile work name moved\n"), 0o600)
	run(t, "ssh", "structure", "approve", "--plan", plan, "-y")
	if _, err := os.Stat(filepath.Join(p.SSHDir, "profiles", "work", "moved")); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr(t, "ssh", "structure", "install", "-y", "--dev-root", t.TempDir()); err == nil {
		t.Error("install ran in dev mode")
	}
}

func keygen(t *testing.T, path, comment string) {
	t.Helper()
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v %s", err, out)
	}
}

// jsonPart is the JSON array in a command's combined output.
func jsonPart(out string) []byte {
	if i := strings.Index(out, "["); i >= 0 {
		return []byte(out[i:])
	}
	return []byte(out)
}
