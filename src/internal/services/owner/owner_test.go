package owner

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/seal"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

// home is a dev sandbox, so the key goes to the file store inside it and
// never to the real keychain.
func home(t *testing.T) paths.Paths {
	t.Helper()
	if _, err := MachineID(); err != nil {
		t.Skipf("no machine id here: %v", err)
	}
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, paths.DevSSHDir), ConfigDir: filepath.Join(root, paths.DevConfigDir), DevRoot: root}
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, p.Manifest(), `{"profiles":{}}`)
	return p
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestADevHomeKeepsItsKeyInsideTheSandbox(t *testing.T) {
	p := home(t)
	if got := StoreFor(p).Name(); got != StoreFile {
		t.Fatalf("store in dev mode: %s", got)
	}
	c, err := Init(p, t0)
	if err != nil {
		t.Fatal(err)
	}
	if c.Store != StoreFile {
		t.Fatalf("recorded store %s", c.Store)
	}
	if _, err := os.Stat(filepath.Join(p.StateDir(), "owner.key")); err != nil {
		t.Fatalf("key file: %v", err)
	}
	if _, err := Init(p, t0); !errors.Is(err, ErrSetUp) {
		t.Fatalf("second Init: %v", err)
	}
}

// The whole point: a change made behind authama's back is named, and a
// reseal (what authama does after its own operations) accepts the new state.
func TestAnOutsideEditIsDetectedUntilResealed(t *testing.T) {
	p := home(t)
	if !Verify(p).Clean() {
		t.Fatal("a home with no owner is not clean")
	}
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if st := Verify(p); !st.Clean() {
		t.Fatalf("fresh seal: %s", st.Describe())
	}
	write(t, p.Manifest(), `{"profiles":{"x":{}}}`)
	write(t, p.Tokens(), `{}`)
	st := Verify(p)
	if st.Clean() || len(st.Report.Findings) != 2 {
		t.Fatalf("after edits: %+v", st)
	}
	got := map[string]seal.Status{}
	for _, f := range st.Report.Findings {
		got[f.Name] = f.Status
	}
	if got["manifest.json"] != seal.Changed || got["tokens.json"] != seal.Appeared {
		t.Fatalf("findings %v; %s", got, st.Describe())
	}
	if err := Reseal(p, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st := Verify(p); !st.Clean() {
		t.Fatalf("after reseal: %s", st.Describe())
	}
}

func TestEditingTheSealItselfIsDetected(t *testing.T) {
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	// Point one sealed entry at "absent", as someone hiding a new file would.
	b, _ := os.ReadFile(SealPath(p))
	edited := regexp.MustCompile(`"manifest.json": "[0-9a-f]+"`).ReplaceAllString(string(b), `"manifest.json": ""`)
	if edited == string(b) {
		t.Fatal("the test's edit matched nothing")
	}
	write(t, SealPath(p), edited)
	if Verify(p).Clean() {
		t.Fatal("a damaged seal verified")
	}
	if err := os.Remove(SealPath(p)); err != nil {
		t.Fatal(err)
	}
	if st := Verify(p); st.Clean() || st.Problem == "" {
		t.Fatalf("a missing seal: %+v", st)
	}
}

func TestPINSetCheckChangeResetRemove(t *testing.T) {
	p := home(t)
	first, second, third := testcred.Secret("first"), testcred.Secret("second"), testcred.Secret("third")
	if err := SetPIN(p, "", first, t0); !errors.Is(err, ErrNotSetUp) {
		t.Fatalf("PIN before init: %v", err)
	}
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := SetPIN(p, "", "123456", t0); err == nil {
		t.Fatal("a guessable PIN was accepted")
	}
	if err := SetPIN(p, "", first, t0); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(ConfigPath(p)); len(b) == 0 || containsStr(string(b), first) {
		t.Fatal("owner.json holds the PIN in clear")
	}
	if err := CheckPIN(p, first, t0); err != nil {
		t.Fatalf("right PIN: %v", err)
	}
	if err := SetPIN(p, second, third, t0); !errors.Is(err, ErrWrongPIN) {
		t.Fatalf("change with the wrong current PIN: %v", err)
	}
	if err := SetPIN(p, first, second, t0); err != nil {
		t.Fatal(err)
	}
	// Forgotten: reset needs no old PIN on the bound account and machine.
	if err := ResetPIN(p, third, t0); err != nil {
		t.Fatal(err)
	}
	if err := CheckPIN(p, third, t0); err != nil {
		t.Fatalf("after reset: %v", err)
	}
	if err := RemovePIN(p, third, t0); err != nil {
		t.Fatal(err)
	}
	if err := CheckPIN(p, third, t0); !errors.Is(err, ErrNoPIN) {
		t.Fatalf("after remove: %v", err)
	}
	// Every PIN change reseals, so owner.json's own edits are not "outside".
	if st := Verify(p); !st.Clean() {
		t.Fatalf("after PIN changes: %s", st.Describe())
	}
}

func TestWrongPINsLockOutWithAGrowingWait(t *testing.T) {
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	right, wrong := testcred.Secret("right"), testcred.Secret("wrong")
	if err := SetPIN(p, "", right, t0); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < FreeAttempts; i++ {
		if err := CheckPIN(p, wrong, t0); !errors.Is(err, ErrWrongPIN) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	var locked LockedError
	if err := CheckPIN(p, wrong, t0); !errors.As(err, &locked) || !locked.Until.Equal(t0.Add(LockoutBase)) {
		t.Fatalf("attempt %d: %v", FreeAttempts, err)
	}
	// Even the right PIN waits out the lockout.
	if err := CheckPIN(p, right, t0.Add(time.Second)); !errors.As(err, &locked) {
		t.Fatalf("right PIN during lockout: %v", err)
	}
	if err := CheckPIN(p, right, t0.Add(LockoutBase)); err != nil {
		t.Fatalf("right PIN after the wait: %v", err)
	}
	if lockout(FreeAttempts+1) != 2*LockoutBase || lockout(100) != LockoutMax {
		t.Fatalf("lockout curve: %v %v", lockout(FreeAttempts+1), lockout(100))
	}
}

func TestForgetUnbindsAndKeepsTheStateFiles(t *testing.T) {
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := Forget(p); err != nil {
		t.Fatal(err)
	}
	if c, _ := Load(p); c != nil {
		t.Fatal("owner.json survived Forget")
	}
	if _, err := os.Stat(p.Manifest()); err != nil {
		t.Fatalf("Forget removed the manifest: %v", err)
	}
	if !Verify(p).Clean() {
		t.Fatal("a forgotten home is not clean")
	}
}

// A seal made on another account or machine is never re-made silently.
func TestResealRefusesAHomeBoundElsewhere(t *testing.T) {
	p := home(t)
	c, err := Init(p, t0)
	if err != nil {
		t.Fatal(err)
	}
	c.Binding = seal.Binding{User: "someone", UID: "9", Machine: "elsewhere"}.ID()
	if err := save(p, c); err != nil {
		t.Fatal(err)
	}
	if err := Reseal(p, t0); !errors.Is(err, ErrElsewhere) {
		t.Fatalf("Reseal: %v", err)
	}
	if err := ResetPIN(p, testcred.Secret("x"), t0); !errors.Is(err, ErrElsewhere) {
		t.Fatalf("ResetPIN elsewhere: %v", err)
	}
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

// A PIN is only worth setting where it can be reset: Recoverable says so
// while the key is readable, and not once it is gone.
func TestRecoverableNeedsTheKey(t *testing.T) {
	p := home(t)
	if err := Recoverable(p); !errors.Is(err, ErrNotSetUp) {
		t.Fatalf("unbound: %v", err)
	}
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := Recoverable(p); err != nil {
		t.Fatalf("bound: %v", err)
	}
	if err := (fileStore{p}).Remove(); err != nil {
		t.Fatal(err)
	}
	if err := Recoverable(p); err == nil {
		t.Fatal("no key, and still called recoverable")
	}
}

// Forget removes owner.json first, so a home is never left bound without
// the key that resets its PIN.
func TestForgetUnbindsBeforeRemovingTheKey(t *testing.T) {
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	// The key is already gone (removed by hand): forget still unbinds.
	_ = (fileStore{p}).Remove()
	if err := Forget(p); err != nil {
		t.Fatalf("forget with the key missing: %v", err)
	}
	if c, _ := Load(p); c != nil {
		t.Fatal("still bound")
	}
}
