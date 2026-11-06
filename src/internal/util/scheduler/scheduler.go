// Package scheduler installs authama's daily jobs - `authama ssh audit --notify`
// (Label) and `authama ssh vm sync --quiet --yes` (VMSyncLabel) -
// ported from the platforms/*.install_scheduler layer: a launchd agent on macOS, a
// systemd --user timer (else cron) on Linux, a schtasks task on Windows. The job
// fires at 09:00 daily. Pure template/quoting helpers live here; the OS-specific
// install lives in build-tagged files.
package scheduler

import (
	"strconv"
	"strings"
)

// Label is the scheduler job label/name.
const Label = "authama.expiry"

// LegacyLabel is the label releases before 0.1.2 installed the job under.
// Installing under Label removes a job left under it, so the two never both
// fire; nothing else reads it.
const LegacyLabel = "ssh_manager.expiry"

// VMSyncLabel is the job `authama ssh vm sync --install-schedule` installs.
const VMSyncLabel = "authama.vmsync"

// job is what each label's job is called, and when it runs.
type job struct{ description, at string }

var jobs = map[string]job{
	Label:       {"authama key-expiry notifier", notifyHour},
	LegacyLabel: {"authama key-expiry notifier", notifyHour},
	VMSyncLabel: {"authama VM host sync", vmSyncHour},
}

func jobFor(label string) job {
	if j, ok := jobs[label]; ok {
		return j
	}
	return job{"authama " + label, notifyHour}
}

// hourMinute splits "HH:MM" into the numbers launchd and cron want ("9", "30").
func hourMinute(at string) (string, string) {
	h, m, _ := strings.Cut(at, ":")
	hi, _ := strconv.Atoi(h)
	mi, _ := strconv.Atoi(m)
	return strconv.Itoa(hi), strconv.Itoa(mi)
}

const plistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>%LABEL%</string>
    <key>ProgramArguments</key>
    <array>
%ARGS%
    </array>
    <key>StartCalendarInterval</key>
    <dict><key>Hour</key><integer>%HOUR%</integer><key>Minute</key><integer>%MINUTE%</integer></dict>
    <key>RunAtLoad</key><false/>
</dict>
</plist>
`

const serviceTmpl = `[Unit]
Description=%DESCRIPTION%

[Service]
Type=oneshot
ExecStart=%COMMAND%
`

// buildPlist renders the launchd plist for a command (label + argv array).
func buildPlist(label, command string) string {
	var args []string
	for _, tok := range shlexSplit(command) {
		args = append(args, "        <string>"+xmlEscape(tok)+"</string>")
	}
	h, m := hourMinute(jobFor(label).at)
	p := strings.NewReplacer("%LABEL%", label, "%HOUR%", h, "%MINUTE%", m).Replace(plistTmpl)
	return strings.ReplaceAll(p, "%ARGS%", strings.Join(args, "\n"))
}

// buildService renders the expiry notifier's systemd .service unit.
func buildService(command string) string { return buildServiceFor(Label, command) }

// buildServiceFor renders the systemd .service unit for label (% escaped for ExecStart).
func buildServiceFor(label, command string) string {
	s := strings.ReplaceAll(serviceTmpl, "%DESCRIPTION%", jobFor(label).description)
	return strings.ReplaceAll(s, "%COMMAND%", strings.ReplaceAll(command, "%", "%%"))
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// shlexSplit is a minimal POSIX shell-word splitter (quotes + backslash).
func shlexSplit(s string) []string {
	var args []string
	var cur strings.Builder
	inWord := false
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if quote == '"' && c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inWord = true
		case c == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
			inWord = true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		args = append(args, cur.String())
	}
	return args
}
