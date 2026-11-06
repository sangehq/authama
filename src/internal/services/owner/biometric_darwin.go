//go:build darwin

package owner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// TouchIDHelper is the helper that asks macOS for Touch ID. authama is built
// without cgo and cannot call LocalAuthentication itself.
const TouchIDHelper = "authama-touchid"

// touchIDHelperPath finds the helper: beside the authama binary (a source
// build, a symlinked install, Authama.app/Contents/MacOS), in the app's
// Helpers folder, then on PATH. Never the current directory alone.
var touchIDHelperPath = func() (string, bool) {
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		dir := filepath.Dir(exe)
		for _, p := range []string{filepath.Join(dir, TouchIDHelper), filepath.Join(dir, "..", "Helpers", TouchIDHelper)} {
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
				return p, true
			}
		}
	}
	if p, err := exec.LookPath(TouchIDHelper); err == nil && filepath.IsAbs(p) {
		return p, true
	}
	return "", false
}

// touchIDReason maps the helper's exit status to why Touch ID cannot be used.
func touchIDReason(code int) string {
	switch code {
	case 2:
		return "this Mac has no Touch ID available here (none, no login password, or not an interactive session)"
	case 3:
		return "no finger is enrolled; add one in System Settings > Touch ID & Password"
	case 4:
		return "Touch ID was cancelled"
	case 5:
		return "Touch ID is locked after too many tries; unlock the Mac with its password to reset it"
	case 64:
		return "the Touch ID helper did not understand the request"
	}
	return fmt.Sprintf("the Touch ID helper failed (status %d)", code)
}

// BiometricAvailable reports whether Touch ID can stand in for the PIN: the
// helper is installed, this is not an SSH session, and macOS says a finger is
// enrolled and Touch ID can be asked for.
func BiometricAvailable() (bool, string) {
	if remoteSession() {
		return false, "this is an SSH session, where Touch ID cannot be asked for"
	}
	helper, ok := touchIDHelperPath()
	if !ok {
		return false, TouchIDHelper + " is not installed beside authama (`make build` builds it on a Mac with Xcode's command line tools)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, err := runStatus(ctx, helper, []string{"--check"}, nil)
	if err != nil {
		return false, "the Touch ID helper could not run: " + err.Error()
	}
	if code != 0 {
		return false, touchIDReason(code)
	}
	return true, ""
}

// VerifyBiometric asks for Touch ID, for up to a minute.
func VerifyBiometric(ctx context.Context) error {
	helper, ok := touchIDHelperPath()
	if !ok {
		return errors.New(TouchIDHelper + " is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	code, err := runStatus(ctx, helper, []string{"unlock authama"}, nil)
	if err != nil {
		return fmt.Errorf("no answer from Touch ID: %w", err)
	}
	if code != 0 {
		return errors.New(touchIDReason(code))
	}
	return nil
}
