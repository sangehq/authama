//go:build darwin

package scheduler

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Install writes a launchd agent that runs command daily at 09:00 and (re)loads it.
func Install(command, label string) error {
	dest, err := plistPath(label)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, []byte(buildPlist(label, command)), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", dest).Run() // ignore (first install)
	return exec.Command("launchctl", "load", dest).Run()
}

// Remove unloads and deletes the launchd agent for label, if there is one.
func Remove(label string) {
	dest, err := plistPath(label)
	if err != nil {
		return
	}
	if _, err := os.Stat(dest); err != nil {
		return
	}
	_ = exec.Command("launchctl", "unload", dest).Run()
	_ = os.Remove(dest)
}

func plistPath(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}
