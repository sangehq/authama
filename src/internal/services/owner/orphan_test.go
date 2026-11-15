package owner

import (
	"errors"
	"os"
	"testing"

	"github.com/sangehq/authama/src/internal/util/testcred"
)

// Deleting owner.json used to switch the seal check to "clean" and the PIN
// off while the seal stayed in place (review R-SEC-02). A home that still
// carries what binding left is now a Problem, the PIN check refuses rather
// than reporting a home that was never set up, and forget finishes it.
func TestAMissingOwnerConfigWithASealIsAProblem(t *testing.T) {
	t.Setenv(StoreEnv, StoreFile)
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := SetPIN(p, "", testcred.Secret("owner"), t0); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ConfigPath(p)); err != nil {
		t.Fatal(err)
	}
	if !Orphaned(p) {
		t.Fatal("a home with a seal and no owner.json is not reported orphaned")
	}
	st := Verify(p)
	if st.Clean() || st.Problem == "" || !st.SetUp {
		t.Fatalf("verify: clean=%v setUp=%v problem=%q", st.Clean(), st.SetUp, st.Problem)
	}
	if err := CheckPIN(p, testcred.Secret("owner"), t0); !errors.Is(err, ErrOrphaned) {
		t.Fatalf("PIN check on an orphaned home: %v, want ErrOrphaned", err)
	}
}

func TestForgetFinishesAnOrphanedHome(t *testing.T) {
	t.Setenv(StoreEnv, StoreFile)
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ConfigPath(p)); err != nil {
		t.Fatal(err)
	}
	if err := Forget(p); err != nil {
		t.Fatalf("forget on an orphaned home: %v", err)
	}
	if Orphaned(p) {
		t.Error("still orphaned after forget")
	}
	for _, f := range []string{SealPath(p), attemptsPath(p), (fileStore{p}).path()} {
		if _, err := os.Lstat(f); err == nil {
			t.Errorf("%s left behind", f)
		}
	}
	if st := Verify(p); !st.Clean() || st.SetUp {
		t.Errorf("after forget: clean=%v setUp=%v %q", st.Clean(), st.SetUp, st.Problem)
	}
}

// A home that was never bound is not orphaned, and forget still says so.
func TestANeverBoundHomeIsNotOrphaned(t *testing.T) {
	p := home(t)
	if Orphaned(p) {
		t.Fatal("a fresh home is reported orphaned")
	}
	if st := Verify(p); !st.Clean() {
		t.Fatalf("fresh home: %q", st.Problem)
	}
	if err := Forget(p); !errors.Is(err, ErrNotSetUp) {
		t.Fatalf("forget on a fresh home: %v", err)
	}
}
