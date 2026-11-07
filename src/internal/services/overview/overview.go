// Package overview is the state of an authama home at a glance: where it
// operates, what it holds, and what needs attention, each problem paired with
// the command that fixes it. The web overview reads it, and offers each fix.
package overview

import (
	"fmt"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/notifier"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Row is one line of the state block. A row with a fix is a problem; Fix is
// the command that resolves it, so the report and the remedy are never in two
// different places.
type Row struct {
	Label string
	Value string
	Fix   string // empty when the row is just a fact
}

// Rows assembles the state block the web overview shows: one place decides
// what needs attention. Every lookup is best-effort: a banner
// that fails to render because one probe errored would make the tool unusable
// for the exact person most likely to need it - someone whose ~/.ssh is broken.
func Rows(p paths.Paths, m *manifest.Manifest, now time.Time) []Row {
	rep := doctor.New(p, m, platform.EmitUseKeychain()).Run(false)

	home := p.ConfigDir
	if rep.ProvidersSource != "" {
		home += "   (providers: " + rep.ProvidersSource + ")"
	}
	rows := []Row{}
	// First row, and marked, when sandboxed. Someone who forgets they passed
	// --dev-root and reconciles will wonder why their real config did not
	// change; someone who thinks they passed it and did not has the opposite,
	// worse problem. Neither is possible if the screen says which it is.
	if p.IsDev() {
		rows = append(rows, Row{
			Label: "DEV MODE", Value: "sandboxed under " + p.DevRoot, Fix: "not your real ~/.ssh"})
	}
	rows = append(rows,
		Row{Label: "home", Value: home},
		Row{Label: "ssh", Value: p.SSHDir},
	)

	// Counts, or the first-run state. "none yet" plus the command that creates
	// one is the whole of what a new user needs from this screen.
	if m == nil {
		rows = append(rows, Row{Label: "manifest", Value: "none yet", Fix: "authama init"})
	} else {
		rows = append(rows, Row{Label: "manifest", Value: manifestCounts(m)})
	}

	rows = append(rows, Row{Label: "agent", Value: rep.AgentStatus})

	if m != nil {
		if rep.ConfigInSync {
			rows = append(rows, Row{Label: "rendered", Value: "config in sync"})
		} else {
			rows = append(rows, Row{
				Label: "rendered", Value: "drifted from the manifest", Fix: "authama ssh reconcile"})
		}
		rows = append(rows, pinRow(rep, m))
	}

	if n := len(rep.PermIssues); n > 0 {
		rows = append(rows, Row{
			Label: "perms", Value: fmt.Sprintf("%s with the wrong mode", plural(n, "path", "paths")),
			Fix: "authama doctor --fix"})
	}
	if !rep.Dangling.OK() {
		rows = append(rows, Row{
			Label: "keys", Value: fmt.Sprintf("%s need attention", plural(len(rep.Dangling.Findings), "key", "keys")),
			Fix: "authama doctor"})
	}
	if n := len(rep.Dangling.Reserved); n > 0 {
		names := make([]string, 0, n)
		for _, r := range rep.Dangling.Reserved {
			names = append(names, r.Name)
		}
		row := Row{Label: "reserved", Value: fmt.Sprintf("%s kept for later: %s", plural(n, "key", "keys"), strings.Join(names, ", "))}
		if bad := len(rep.Dangling.ReservedProblems()); bad > 0 {
			row.Value = fmt.Sprintf("%s no longer match their reservation", plural(bad, "key", "keys"))
			row.Fix = "authama doctor"
		}
		rows = append(rows, row)
	}
	if !rep.Preflight.OK() {
		rows = append(rows, Row{
			Label: "deps", Value: "a required OpenSSH tool is missing", Fix: "authama doctor"})
	}
	if rep.StrandedLegacyHome != "" {
		rows = append(rows, Row{
			Label: "legacy", Value: "an old config home is still on disk", Fix: "authama migrate"})
	}
	if rep.StrandedKeyRoot != "" {
		rows = append(rows, Row{
			Label: "keys", Value: "keys are left in ~/.ssh/profiles/, outside the key root", Fix: "authama doctor"})
	}

	// Expiry last: it is a schedule rather than a fault, and it is the one row
	// that is normally absent.
	if m != nil {
		if due := notifier.New(p, m).Banner(now); due != "" {
			rows = append(rows, Row{
				Label: "rotation", Value: firstLine(due), Fix: "authama ssh rotate"})
		}
	}
	return rows
}

// pinRow reports trust-store coverage as a fraction, because "6 of 7" says both
// how much is done and how much is left; a bare count of unpinned hosts reads as
// fine when it is 0 and as meaningless when it is 3.
func pinRow(rep doctor.Report, m *manifest.Manifest) Row {
	total := hostCount(m)
	unpinned := len(rep.UnpinnedHosts)
	switch {
	case total == 0:
		return Row{Label: "pins", Value: "no hosts declared"}
	case unpinned == 0:
		return Row{Label: "pins", Value: fmt.Sprintf("all %d pinned", total)}
	default:
		return Row{
			Label: "pins",
			Value: fmt.Sprintf("%d of %d pinned", total-unpinned, total),
			Fix:   "authama ssh knownhosts pin",
		}
	}
}

func manifestCounts(m *manifest.Manifest) string {
	profiles := len(m.ProfileNames())
	keys := 0
	if refs, err := m.KeyRefs(); err == nil {
		keys = len(refs)
	}
	return fmt.Sprintf("%s, %s, %s",
		plural(profiles, "profile", "profiles"),
		plural(hostCount(m), "host", "hosts"),
		plural(keys, "key", "keys"))
}

func hostCount(m *manifest.Manifest) int {
	if m == nil {
		return 0
	}
	n := 0
	for _, name := range m.ProfileNames() {
		n += len(m.Profiles[name].Hosts)
	}
	return n
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return s[:i]
	}
	return s
}
