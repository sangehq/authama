//go:build !linux

package owner

import (
	"context"
	"errors"
)

// BiometricAvailable: on macOS and Windows a fingerprint (Touch ID, Windows
// Hello) is only offered to a signed application through the platform's own
// API, which a cgo-free command-line binary cannot call. Until authama ships
// a signed helper for it, the PIN is the only unlock there.
func BiometricAvailable() (bool, string) {
	return false, "Touch ID and Windows Hello need a signed helper authama does not ship yet; the PIN is used"
}

// VerifyBiometric is never offered where BiometricAvailable is false.
func VerifyBiometric(context.Context) error {
	return errors.New("no fingerprint unlock on this platform")
}
