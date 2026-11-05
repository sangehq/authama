package owner

import (
	"context"
	"errors"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// BiometricAvailable reports whether fprintd can verify a fingerprint for this
// user: the tool is installed and a finger is enrolled.
func BiometricAvailable() (bool, string) {
	if _, err := exec.LookPath("fprintd-verify"); err != nil {
		return false, "fprintd is not installed (fprintd-verify was not found)"
	}
	out, err := exec.Command("fprintd-list", currentUserName()).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Fingerprints for user") || strings.Contains(string(out), "no fingers enrolled") {
		return false, "no finger is enrolled; enrol one with fprintd-enroll"
	}
	return true, ""
}

// VerifyBiometric asks fprintd for a fingerprint match, for up to 30 seconds.
func VerifyBiometric(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fprintd-verify").CombinedOutput()
	if err == nil && strings.Contains(string(out), "verify-match") {
		return nil
	}
	return errors.New("the fingerprint did not match")
}

func currentUserName() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}
