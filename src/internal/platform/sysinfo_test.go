package platform

import (
	"runtime"
	"testing"
	"time"
)

func TestOSReleaseNamesTheDistribution(t *testing.T) {
	var si SystemInfo
	osRelease(&si, "NAME=\"Ubuntu\"\nVERSION=\"24.04.3 LTS (Noble Numbat)\"\nVERSION_ID=\"24.04\"\nID=ubuntu\n")
	if si.OS != "Ubuntu" || si.OSVersion != "24.04.3 LTS (Noble Numbat)" {
		t.Errorf("got %q %q", si.OS, si.OSVersion)
	}
	si = SystemInfo{}
	osRelease(&si, "NAME=Arch Linux\nBUILD_ID=rolling\n")
	if si.OS != "Arch Linux" || si.OSBuild != "rolling" || si.OSVersion != "" {
		t.Errorf("got %q %q %q", si.OS, si.OSVersion, si.OSBuild)
	}
	si = SystemInfo{}
	osRelease(&si, "")
	if si.OS != "Linux" {
		t.Errorf("no os-release: OS = %q, want Linux", si.OS)
	}
}

func TestCPUInfoCountsPhysicalCores(t *testing.T) {
	// Two sockets of two cores, hyperthreaded: eight processors, four cores.
	var text string
	for p := range 8 {
		socket, core := p/4, (p%4)/2
		text += "processor\t: " + string(rune('0'+p)) + "\nmodel name\t: Intel(R) Xeon(R) Gold 6338\n" +
			"physical id\t: " + string(rune('0'+socket)) + "\ncore id\t\t: " + string(rune('0'+core)) + "\n\n"
	}
	var si SystemInfo
	cpuInfo(&si, text)
	if si.CPU != "Intel(R) Xeon(R) Gold 6338" || si.Cores != 4 {
		t.Errorf("got %q, %d cores", si.CPU, si.Cores)
	}
	si = SystemInfo{}
	cpuInfo(&si, "processor\t: 0\nBogoMIPS\t: 108.00\nModel\t\t: Raspberry Pi 5 Model B Rev 1.0\n")
	if si.CPU != "Raspberry Pi 5 Model B Rev 1.0" || si.Cores != 0 {
		t.Errorf("arm board: got %q, %d cores", si.CPU, si.Cores)
	}
}

// On a first-class OS the live reading names the OS, the processor and the
// memory; elsewhere it at least names the OS and counts the threads.
func TestSystemReadsThisComputer(t *testing.T) {
	si := System()
	if si.OS == "" || si.Arch != runtime.GOARCH || si.Threads < 1 {
		t.Fatalf("System() = %+v", si)
	}
	if !FirstClass() {
		return
	}
	if si.Memory < 64<<20 {
		t.Errorf("memory %d bytes is implausible", si.Memory)
	}
	if si.Kernel == "" {
		t.Error("no kernel")
	}
	// A sandbox may refuse the boot time (macOS's does): unknown is allowed,
	// a boot in the future is not.
	if !si.Booted.IsZero() && si.Booted.After(time.Now()) {
		t.Errorf("boot time %v is in the future", si.Booted)
	}
	if IsMacOS() && (si.CPU == "" || si.OSVersion == "" || si.Cores < 1) {
		t.Errorf("macOS: %+v", si)
	}
}
