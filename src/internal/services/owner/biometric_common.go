//go:build darwin || windows

package owner

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

// remoteSession reports whether this process runs in an SSH session, where a
// fingerprint prompt would appear on a screen nobody at this terminal sees
// (or nowhere), so the PIN is asked instead.
func remoteSession() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != ""
}

// runStatus runs a command and returns its exit status, or an error when it
// could not be started or was stopped. A seam for the tests.
var runStatus = func(ctx context.Context, name string, args []string, env []string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, nil
	case errors.As(err, &exit) && ctx.Err() == nil:
		return exit.ExitCode(), nil
	}
	return -1, err
}
