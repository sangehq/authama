//go:build windows

package platform

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func fillSystem(si *SystemInfo) {
	str := func(k registry.Key, name string) string {
		v, _, err := k.GetStringValue(name)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(v)
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		si.OS = str(k, "ProductName")
		si.OSVersion = str(k, "DisplayVersion")
		build := str(k, "CurrentBuild")
		if ubr, _, err := k.GetIntegerValue("UBR"); err == nil && build != "" {
			build += "." + strconv.FormatUint(ubr, 10)
		}
		si.OSBuild = build
		// Windows 11 still says "Windows 10" in ProductName; build 22000 is 11.
		if strings.HasPrefix(si.OS, "Windows 10") && buildNumber(str(k, "CurrentBuild")) >= 22000 {
			si.OS = "Windows 11" + strings.TrimPrefix(si.OS, "Windows 10")
		}
		_ = k.Close()
	}
	if si.OS == "" {
		si.OS = "Windows"
	}
	v := windows.RtlGetVersion()
	si.Kernel = fmt.Sprintf("Windows NT %d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		si.CPU = str(k, "ProcessorNameString")
		_ = k.Close()
	}
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\BIOS`, registry.QUERY_VALUE); err == nil {
		si.Vendor = str(k, "SystemManufacturer")
		si.Model = str(k, "SystemProductName")
		_ = k.Close()
	}
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r != 0 {
		si.Memory = ms.TotalPhys
	}
	si.Booted = time.Now().Add(-time.Duration(windows.DurationSinceBoot()))
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryStatusEx is MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// buildNumber is a CurrentBuild value as a number, 0 when it is not one.
func buildNumber(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
