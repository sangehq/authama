//go:build !linux && !darwin && !windows

package owner

import (
	"errors"
	"os"
	"strings"
)

// MachineID on the BSDs is the host UUID the kernel or rc keeps in
// /etc/hostid; elsewhere there is none authama knows how to read.
func MachineID() (string, error) {
	if b, err := os.ReadFile("/etc/hostid"); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
	}
	return "", errors.New("this platform has no machine id authama can read")
}
