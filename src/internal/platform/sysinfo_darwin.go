//go:build darwin

package platform

import (
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func fillSystem(si *SystemInfo) {
	str := func(name string) string {
		v, err := unix.Sysctl(name)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(v)
	}
	num := func(name string) int {
		v, err := unix.SysctlUint32(name)
		if err != nil {
			return 0
		}
		return int(v)
	}
	si.OS = "macOS"
	si.OSVersion = str("kern.osproductversion")
	si.OSBuild = str("kern.osversion")
	if r := str("kern.osrelease"); r != "" {
		si.Kernel = "Darwin " + r
	}
	si.Vendor = "Apple"
	si.Model = str("hw.model")
	si.CPU = str("machdep.cpu.brand_string")
	si.Cores = num("hw.physicalcpu")
	// Apple silicon names its two clusters; an Intel Mac has no perflevel1.
	if eff := num("hw.perflevel1.physicalcpu"); eff > 0 {
		si.PerfCores = num("hw.perflevel0.physicalcpu")
		si.EffCores = eff
	}
	if m, err := unix.SysctlUint64("hw.memsize"); err == nil {
		si.Memory = m
	}
	if tv, err := unix.SysctlTimeval("kern.boottime"); err == nil && tv.Sec > 0 {
		si.Booted = time.Unix(tv.Sec, int64(tv.Usec)*1000)
	}
}
