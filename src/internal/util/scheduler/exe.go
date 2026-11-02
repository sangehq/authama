package scheduler

import (
	"os"
	"os/exec"
	"strings"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/platform"
)

// Exe is the quoted invocation a scheduled job runs: the authama on PATH if
// present, then the sshmgr compatibility binary, else this binary; quoted so a
// path with spaces stays one token (double quotes on Windows, shell-quote on
// POSIX). Mirrors facade._scheduler_exe.
func Exe() string {
	exe := ""
	for _, name := range brand.Commands() {
		if p, err := exec.LookPath(name); err == nil {
			exe = p
			break
		}
	}
	if exe == "" {
		if self, e := os.Executable(); e == nil {
			exe = self
		} else {
			exe = brand.Command
		}
	}
	if platform.IsWindows() {
		return `"` + exe + `"`
	}
	return ShellQuote(exe)
}

// ShellQuote quotes a string for POSIX shells (shlex.quote): bare if safe, else
// single-quoted with embedded quotes escaped.
func ShellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\$`&|;<>()*?[]{}~#!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// VMSyncCommand is the command the scheduled VM sync runs.
func VMSyncCommand() string { return Exe() + " ssh vm sync --quiet --yes" }
