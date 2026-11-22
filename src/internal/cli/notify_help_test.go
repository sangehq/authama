package cli

import (
	"strings"
	"testing"
)

// notify send, ping and doctor hand their flags to notifykit, yet --help on
// them is help: it neither shows a notification nor probes the desktop.
func TestNotifyVerbsHonourHelp(t *testing.T) {
	editHome(t)
	for _, verb := range []string{"send", "ping", "doctor"} {
		out := run(t, "ssh", "notify", verb, "--help")
		if !strings.Contains(out, "authama ssh notify "+verb) || strings.Contains(out, "backend:") {
			t.Errorf("notify %s --help:\n%s", verb, out)
		}
	}
}

// doctor takes no flags: an unknown one is refused rather than ignored, and
// its help does not list send's flags (found by the QA walk).
func TestNotifyDoctorRefusesAFlag(t *testing.T) {
	editHome(t)
	if _, err := runErr(t, "ssh", "notify", "doctor", "--title", "x"); err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("notify doctor --title = %v, want an unknown-flag error", err)
	}
	if out := run(t, "ssh", "notify", "doctor", "--help"); strings.Contains(out, "-title") {
		t.Errorf("notify doctor --help lists send's flags:\n%s", out)
	}
	if out := run(t, "ssh", "notify", "send", "--help"); !strings.Contains(out, "-icon PATH") || !strings.Contains(out, "go-notifykit") {
		t.Errorf("notify send --help should list every notifykit flag:\n%s", out)
	}
}
