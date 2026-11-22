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
