package tokens

import (
	"regexp"
	"strings"
)

// Shell families for EnvExport.
const (
	ShellPOSIX      = "posix"
	ShellFish       = "fish"
	ShellPowerShell = "powershell"
	ShellCmd        = "cmd"
)

// DetectShell guesses the current shell family from the environment the way
// the Python tool did: PowerShell on Windows when PSModulePath is set, fish
// or POSIX from $SHELL, else cmd/PowerShell on Windows and POSIX elsewhere.
func DetectShell(getenv func(string) string, windows bool) string {
	if getenv("PSModulePath") != "" && windows {
		return ShellPowerShell
	}
	sh := getenv("SHELL")
	if strings.HasSuffix(sh, "fish") {
		return ShellFish
	}
	if sh != "" {
		return ShellPOSIX
	}
	if windows {
		if getenv("PSModulePath") != "" {
			return ShellPowerShell
		}
		return ShellCmd
	}
	return ShellPOSIX
}

var cmdSpecial = regexp.MustCompile(`[&|<>^()"]`)

// EnvExport renders one NAME=value assignment for a shell family.
func EnvExport(name, value, shell string) string {
	switch shell {
	case ShellPowerShell:
		return "$env:" + name + " = '" + strings.ReplaceAll(value, "'", "''") + "'"
	case ShellCmd:
		if strings.Contains(value, `"`) {
			return "set " + name + "=" + cmdSpecial.ReplaceAllString(value, "^$0")
		}
		return `set "` + name + "=" + value + `"`
	case ShellFish:
		v := strings.ReplaceAll(value, `\`, `\\`)
		v = strings.ReplaceAll(v, "'", `\'`)
		return "set -gx " + name + " '" + v + "'"
	}
	return "export " + name + "='" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
