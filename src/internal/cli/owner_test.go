package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/services/owner"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

// ownerHome is editHome bound to an owner, with its key in the file store so
// no test reaches the real keychain.
func ownerHome(t *testing.T) {
	t.Helper()
	if _, err := owner.MachineID(); err != nil {
		t.Skipf("no machine id: %v", err)
	}
	editHome(t)
	t.Setenv(owner.StoreEnv, owner.StoreFile)
	run(t, "ssh", "profile", "add", "work")
	run(t, "owner", "init", "--yes")
}

// authama's own changes are resealed; a change made behind its back is named
// on the next mutating command and is not resealed over.
func TestOwnChangesAreResealedAndOutsideOnesAreNot(t *testing.T) {
	ownerHome(t)
	p, _ := resolvePaths(nil)
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	if st := owner.Verify(p); !st.Clean() {
		t.Fatalf("after authama's own change: %s", st.Describe())
	}
	b, _ := os.ReadFile(p.Manifest())
	if err := os.WriteFile(p.Manifest(), append(b, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	// The warning goes to the process's stderr, which run does not capture;
	// what matters is that the seal still names the edit afterwards.
	run(t, "ssh", "host", "add", "work", "gl", "-H", "gitlab.com", "-u", "git")
	if owner.Verify(p).Clean() {
		t.Fatal("the outside edit was resealed by an unrelated command")
	}
	if _, err := runErr(t, "owner", "verify"); err == nil {
		t.Fatal("owner verify passed a changed manifest")
	}
	run(t, "owner", "reseal", "--yes")
	if st := owner.Verify(p); !st.Clean() {
		t.Fatalf("after reseal: %s", st.Describe())
	}
}

// With a PIN set, a destructive command and a secret read need it, and with
// no terminal to ask on they refuse rather than run unguarded.
func TestAPINGatesDestructiveCommandsAndSecretReads(t *testing.T) {
	ownerHome(t)
	p, _ := resolvePaths(nil)
	if err := owner.SetPIN(p, "", testcred.Secret("owner"), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"ssh", "profile", "delete", "work", "--yes"},
		{"token", "list", "--reveal", "--yes"},
	} {
		out, err := runErr(t, args...)
		if err == nil || !strings.Contains(err.Error(), "a PIN is set") {
			t.Errorf("%v ran without the PIN: %v\n%s", args, err, out)
		}
	}
	// Read-only commands and dry runs are not gated.
	run(t, "ssh", "list")
	if _, err := runErr(t, "ssh", "profile", "delete", "work", "--dry-run"); err != nil && strings.Contains(err.Error(), "a PIN is set") {
		t.Errorf("a dry run asked for the PIN: %v", err)
	}
}

func TestWithoutAPINNothingIsGated(t *testing.T) {
	ownerHome(t)
	run(t, "ssh", "profile", "delete", "work", "--yes")
}
