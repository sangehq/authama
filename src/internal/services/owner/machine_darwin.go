package owner

import (
	"errors"
	"os/exec"
	"regexp"
)

var platformUUID = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([0-9A-Fa-f-]{36})"`)

// MachineID is the hardware UUID macOS reports (System Information's
// "Hardware UUID"), read with ioreg, which every macOS has.
func MachineID() (string, error) {
	out, err := exec.Command("/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", err
	}
	m := platformUUID.FindSubmatch(out)
	if m == nil {
		return "", errors.New("ioreg reported no IOPlatformUUID")
	}
	return string(m[1]), nil
}
