package owner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sangehq/authama/src/internal/core/pin"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/lock"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// FreeAttempts is how many wrong PINs in a row are allowed before each further
// one waits; the wait doubles from LockoutBase up to LockoutMax.
const (
	FreeAttempts = 5
	LockoutBase  = 30 * time.Second
	LockoutMax   = 15 * time.Minute
)

// LockedError is a check refused because of earlier wrong PINs.
type LockedError struct{ Until time.Time }

func (e LockedError) Error() string {
	return fmt.Sprintf("too many wrong PINs; try again after %s", e.Until.Local().Format("15:04:05"))
}

// verifyHook runs just before the hash is checked. Tests only.
var verifyHook func()

type attempts struct {
	Failures int       `json:"failures"`
	Until    time.Time `json:"until,omitempty"`
}

func loadAttempts(p paths.Paths) attempts {
	var a attempts
	if b, err := os.ReadFile(attemptsPath(p)); err == nil {
		_ = json.Unmarshal(b, &a)
	}
	return a
}

func saveAttempts(p paths.Paths, a attempts) error {
	if a.Failures == 0 {
		if err := os.Remove(attemptsPath(p)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	if err := fs.EnsureDir(p.StateDir(), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(attemptsPath(p), string(b)+"\n", 0o600)
}

// lockout is the wait after n consecutive failures.
func lockout(n int) time.Duration {
	if n < FreeAttempts {
		return 0
	}
	d := LockoutBase
	for i := FreeAttempts; i < n && d < LockoutMax; i++ {
		d *= 2
	}
	return min(d, LockoutMax)
}

// CheckPIN checks candidate against the PIN set for the home. Wrong answers
// are counted across processes, in the state home, so restarting the server
// or the command does not reset the wait; a right one clears the count, and
// rehashes a PIN stored at a lower cost than today's.
func CheckPIN(p paths.Paths, candidate string, now time.Time) error {
	c, err := Load(p)
	if err != nil {
		return err
	}
	if c == nil {
		if Orphaned(p) {
			return ErrOrphaned
		}
		return ErrNotSetUp
	}
	if !c.HasPIN() {
		return ErrNoPIN
	}
	// One check at a time, across goroutines and processes: without the lock,
	// parallel guesses each read the same count and all reached the hash.
	release, err := lock.Acquire(filepath.Join(p.StateDir(), "pin.lock"))
	if err != nil {
		return fmt.Errorf("owner: lock the PIN check: %w", err)
	}
	defer release()
	a := loadAttempts(p)
	if now.Before(a.Until) {
		return LockedError{Until: a.Until}
	}
	// Count the guess before checking it, so a check killed mid-verify still
	// counts; a right PIN clears the count below.
	a.Failures++
	if err := saveAttempts(p, a); err != nil {
		return fmt.Errorf("owner: record the PIN attempt: %w", err)
	}
	if verifyHook != nil {
		verifyHook()
	}
	ok, stale, err := pin.Verify(c.PIN, candidate)
	if err != nil {
		return err
	}
	if !ok {
		if d := lockout(a.Failures); d > 0 {
			a.Until = now.Add(d)
		}
		if err := saveAttempts(p, a); err != nil {
			return fmt.Errorf("owner: record the PIN attempt: %w", err)
		}
		if !a.Until.IsZero() && now.Before(a.Until) {
			return errors.Join(ErrWrongPIN, LockedError{Until: a.Until})
		}
		return ErrWrongPIN
	}
	if err := saveAttempts(p, attempts{}); err != nil {
		return fmt.Errorf("owner: clear the PIN attempts: %w", err)
	}
	if stale {
		if cc, key, b, err := unlock(p); err == nil {
			_ = writePIN(p, cc, key, b, candidate, now)
		}
	}
	return nil
}
