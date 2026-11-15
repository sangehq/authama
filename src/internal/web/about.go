package web

import (
	"fmt"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/platform"
)

// AboutView is the about page: what this build is and where it keeps things.
type AboutView struct {
	Name, Summary, Command, Version string
	GoVersion, OS, Arch             string
	Revision, BuiltAt               string
	Modified                        bool
	ConfigHome, StateHome, SSHDir   string
	DevRoot                         string
	Repository, Docs, Issues        string
	User, Host                      string
	Licences                        []licence
	// System is the computer the server runs on: OS, model, processor,
	// memory, uptime. Fields this OS does not expose are empty.
	System platform.SystemInfo
}

// MemoryText is the computer's memory in GB, or "" when unknown.
func (v AboutView) MemoryText() string { return memoryText(v.System.Memory) }

// UptimeText is how long the computer has been up, or "" when unknown.
func (v AboutView) UptimeText() string { return uptimeText(v.System.Uptime()) }

// CoresText says how many cores and threads the processor has, splitting
// Apple silicon's performance and efficiency cores.
func (v AboutView) CoresText() string {
	s := v.System
	out := ""
	switch {
	case s.PerfCores > 0 && s.EffCores > 0:
		out = fmt.Sprintf("%d cores (%d performance, %d efficiency)", s.PerfCores+s.EffCores, s.PerfCores, s.EffCores)
	case s.Cores > 0:
		out = fmt.Sprintf("%d cores", s.Cores)
	}
	if s.Threads > 0 && s.Threads != s.Cores && s.Threads != s.PerfCores+s.EffCores {
		if out != "" {
			out += ", "
		}
		out += fmt.Sprintf("%d threads", s.Threads)
	}
	if out == "" && s.Threads > 0 {
		out = fmt.Sprintf("%d threads", s.Threads)
	}
	return out
}

// memoryText is bytes as GB, the way the OS's own About window says it: whole
// gigabytes when it is a whole number (16 GB), one decimal otherwise.
func memoryText(b uint64) string {
	if b == 0 {
		return ""
	}
	gb := float64(b) / (1 << 30)
	if gb == float64(int64(gb)) {
		return fmt.Sprintf("%d GB", int64(gb))
	}
	return fmt.Sprintf("%.1f GB", gb)
}

// uptimeText is a duration in days, hours and minutes, largest two units.
func uptimeText(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	days, hours, mins := int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour), int(d%time.Hour/time.Minute)
	plural := func(n int, unit string) string {
		if n == 1 {
			return "1 " + unit
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case days > 0:
		return plural(days, "day") + ", " + plural(hours, "hour")
	case hours > 0:
		return plural(hours, "hour") + ", " + plural(mins, "minute")
	default:
		return plural(mins, "minute")
	}
}

func (h systemPages) handleAbout(w http.ResponseWriter, r *http.Request) {
	p := h.s.opts.Engine.Home()
	v := AboutView{Name: brand.Name, Summary: brand.Summary, Command: brand.Command, Version: h.s.opts.Version,
		GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH,
		ConfigHome: p.ConfigDir, StateHome: p.State(), SSHDir: p.SSHDir, DevRoot: p.DevRoot,
		Repository: "https://github.com/sangehq/authama",
		Docs:       brand.DocsURL,
		Issues:     "https://github.com/sangehq/authama/issues",
		User:       h.s.who.user, Host: h.s.who.host, Licences: h.s.licences(), System: platform.System()}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				v.Revision = s.Value
			case "vcs.time":
				v.BuiltAt = s.Value
			case "vcs.modified":
				v.Modified = s.Value == "true"
			}
		}
	}
	h.s.render(w, http.StatusOK, "about", h.s.newView(r, "about", "About", v))
}
