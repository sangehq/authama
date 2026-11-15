package web

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/platform"
)

func TestAboutShowsThisComputer(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/about").Body.String()
	for _, want := range []string{`id="system"`, "This computer", "Operating system", "Hardware", "Architecture", runtime.GOARCH} {
		if !strings.Contains(body, want) {
			t.Errorf("About lacks %q", want)
		}
	}
}

func TestAboutFormatsMemoryUptimeAndCores(t *testing.T) {
	for b, want := range map[uint64]string{0: "", 16 << 30: "16 GB", 64 << 30: "64 GB", 15_900_000_000: "14.8 GB"} {
		if got := memoryText(b); got != want {
			t.Errorf("memoryText(%d) = %q, want %q", b, got, want)
		}
	}
	for d, want := range map[time.Duration]string{
		0: "", 59 * time.Second: "0 minutes", time.Minute: "1 minute",
		2*time.Hour + 5*time.Minute: "2 hours, 5 minutes", 49 * time.Hour: "2 days, 1 hour",
	} {
		if got := uptimeText(d); got != want {
			t.Errorf("uptimeText(%v) = %q, want %q", d, got, want)
		}
	}
	for _, c := range []struct {
		si   platform.SystemInfo
		want string
	}{
		{platform.SystemInfo{Cores: 12, Threads: 12, PerfCores: 8, EffCores: 4}, "12 cores (8 performance, 4 efficiency)"},
		{platform.SystemInfo{Cores: 8, Threads: 16}, "8 cores, 16 threads"},
		{platform.SystemInfo{Cores: 4, Threads: 4}, "4 cores"},
		{platform.SystemInfo{Threads: 2}, "2 threads"},
		{platform.SystemInfo{}, ""},
	} {
		if got := (AboutView{System: c.si}).CoresText(); got != c.want {
			t.Errorf("CoresText(%+v) = %q, want %q", c.si, got, c.want)
		}
	}
}
