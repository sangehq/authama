package owner

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// MachineID is the MachineGuid Windows writes at install time.
func MachineID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", errors.New("MachineGuid is empty")
	}
	return v, nil
}
