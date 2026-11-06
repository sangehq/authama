package platform

import (
	"os"
	"runtime"
	"time"
)

// SystemInfo describes the computer authama runs on, for the About page.
// Every field is best effort: what this OS does not expose, or refuses to say,
// is left empty (zero), and nothing here ever fails.
type SystemInfo struct {
	OS        string // "macOS", "Ubuntu", "Windows 11 Pro"
	OSVersion string // "26.1", "24.04.3 LTS (Noble Numbat)", "24H2"
	OSBuild   string // "25B78", "26100.4652"
	Kernel    string // "Darwin 25.1.0", "Linux 6.8.0-51-generic"
	Arch      string // runtime.GOARCH: "arm64", "amd64"
	Hostname  string
	Model     string // "MacBookPro18,3", "ThinkPad X1 Carbon Gen 11"
	Vendor    string // "Apple", "LENOVO"
	CPU       string // "Apple M1 Pro", "Intel(R) Core(TM) i7-1365U"
	Cores     int    // physical cores, when known
	Threads   int    // logical processors (runtime.NumCPU)
	PerfCores int    // Apple silicon: performance cores
	EffCores  int    // Apple silicon: efficiency cores
	Memory    uint64 // total physical memory, bytes
	Booted    time.Time
}

// System gathers SystemInfo for this computer. It reads the kernel and the
// OS's own records directly (sysctl, /proc, the registry) and runs no
// commands, so it is quick enough to call per request.
func System() SystemInfo {
	si := SystemInfo{Arch: runtime.GOARCH, Threads: runtime.NumCPU()}
	si.Hostname, _ = os.Hostname()
	fillSystem(&si)
	return si
}

// Uptime is how long the computer has been up, rounded to the minute; zero
// when the boot time is unknown.
func (s SystemInfo) Uptime() time.Duration {
	if s.Booted.IsZero() {
		return 0
	}
	return time.Since(s.Booted).Round(time.Minute)
}
