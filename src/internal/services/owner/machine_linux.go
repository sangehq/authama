package owner

import (
	"errors"
	"os"
	"strings"
)

// MachineID is systemd's /etc/machine-id, or D-Bus's copy of it on a system
// without systemd: stable across reboots, and different on every install.
func MachineID() (string, error) {
	for _, f := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(f); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id, nil
			}
		}
	}
	return "", errors.New("no /etc/machine-id or /var/lib/dbus/machine-id")
}
