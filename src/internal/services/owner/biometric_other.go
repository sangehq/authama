//go:build !linux && !darwin && !windows

package owner

import (
	"context"
	"errors"
)

// BiometricAvailable: the BSDs and other platforms have no fingerprint
// service authama talks to; the PIN is the unlock there.
func BiometricAvailable() (bool, string) {
	return false, "this platform has no fingerprint unlock authama can use; the PIN is used"
}

// VerifyBiometric is never offered where BiometricAvailable is false.
func VerifyBiometric(context.Context) error {
	return errors.New("no fingerprint unlock on this platform")
}
