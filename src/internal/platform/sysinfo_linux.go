//go:build linux

package platform

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func fillSystem(si *SystemInfo) {
	osRelease(si, readFile("/etc/os-release"))
	var u unix.Utsname
	if err := unix.Uname(&u); err == nil {
		si.Kernel = unix.ByteSliceToString(u.Sysname[:]) + " " + unix.ByteSliceToString(u.Release[:])
	}
	// DMI on PCs and most servers; the device tree on boards such as a Pi.
	si.Vendor = readFile("/sys/class/dmi/id/sys_vendor")
	si.Model = readFile("/sys/class/dmi/id/product_name")
	if si.Model == "" {
		si.Model = strings.TrimRight(readFile("/proc/device-tree/model"), "\x00")
	}
	cpuInfo(si, readFile("/proc/cpuinfo"))
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err == nil {
		si.Memory = uint64(info.Totalram) * uint64(info.Unit) //nolint:unconvert // the field types differ per GOARCH
		si.Booted = time.Now().Add(-time.Duration(info.Uptime) * time.Second)
	}
}

// readFile is a file's content, trimmed, or "" when it cannot be read.
func readFile(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // fixed system paths
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
