package owner

import (
	"errors"
	"sync"
	"testing"

	"github.com/sangehq/authama/src/internal/util/testcred"
)

// Wrong PINs sent at the same moment are each counted: no more than
// FreeAttempts of them reach the hash before the lockout starts, and every
// one that did is on record. Without a lock around the count, 50 parallel
// guesses were all checked and one was recorded (review R-SEC-01).
func TestConcurrentWrongPINsAllCount(t *testing.T) {
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	right, wrong := testcred.Secret("right"), testcred.Secret("wrong")
	if err := SetPIN(p, "", right, t0); err != nil {
		t.Fatal(err)
	}
	const n = 30
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		evaluated int
		locked    int
	)
	for range n {
		wg.Go(func() {
			err := CheckPIN(p, wrong, t0)
			mu.Lock()
			defer mu.Unlock()
			var l LockedError
			switch {
			case errors.Is(err, ErrWrongPIN):
				evaluated++
			case errors.As(err, &l):
				locked++
			default:
				t.Errorf("unexpected answer: %v", err)
			}
		})
	}
	wg.Wait()
	a := loadAttempts(p)
	if evaluated+locked != n {
		t.Fatalf("answers: %d evaluated + %d locked, want %d in all", evaluated, locked, n)
	}
	if evaluated != FreeAttempts {
		t.Errorf("%d guesses reached the hash, want exactly %d", evaluated, FreeAttempts)
	}
	if a.Failures != evaluated {
		t.Errorf("recorded %d failures for %d evaluated guesses", a.Failures, evaluated)
	}
	if !t0.Before(a.Until) {
		t.Errorf("no lockout recorded after %d failures", a.Failures)
	}
}

// A check that dies mid-verify still counts: the failure is written before
// the hash is checked, so killing the process cannot skip the count.
func TestAFailureIsRecordedBeforeTheHashIsChecked(t *testing.T) {
	p := home(t)
	if _, err := Init(p, t0); err != nil {
		t.Fatal(err)
	}
	if err := SetPIN(p, "", testcred.Secret("right"), t0); err != nil {
		t.Fatal(err)
	}
	seen := -1
	verifyHook = func() { seen = loadAttempts(p).Failures }
	t.Cleanup(func() { verifyHook = nil })
	_ = CheckPIN(p, testcred.Secret("wrong"), t0)
	if seen != 1 {
		t.Errorf("failures on disk while the hash was checked: %d, want 1", seen)
	}
	verifyHook = func() { seen = loadAttempts(p).Failures }
	if err := CheckPIN(p, testcred.Secret("right"), t0); err != nil {
		t.Fatal(err)
	}
	if got := loadAttempts(p).Failures; got != 0 {
		t.Errorf("a right PIN left %d failures on record", got)
	}
}
