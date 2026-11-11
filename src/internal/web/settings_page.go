package web

import (
	"strings"

	"github.com/simtabi/authama/src/internal/services/settings"
)

// settingField is one setting on the Settings page: its value in effect and
// where that came from, and what its edit form needs - its kind, choices,
// floor and description - and whether authama.json holds it (so it can go
// back to its default) or the environment sets it (which wins).
type settingField struct {
	SettingRow
	Spec       settings.Spec
	InFile     bool
	Overridden bool
}

// settingSection is one card of settings: a section of authama.json.
type settingSection struct {
	Key, Title, Summary string
	Fields              []settingField
}

// sectionTitles names each section of authama.json for people, with a line
// on what it holds, in the order the page shows them.
var sectionTitles = []struct{ Key, Title, Summary string }{
	{"backup", "Backups", "The encrypted backups written before anything destructive, and how many snapshots are kept."},
	{"ssh", "SSH", "How new hosts' keys are trusted."},
	{"git", "Git", "How `ssh git` rewrites remotes."},
	{"tokens", "Auth tokens", "How credentials some tools keep elsewhere are handled."},
	{"history", "History", "The record of every change."},
	{"notifications", "Notifications", "Desktop notices about keys."},
	{"vm", "VMs", "Which hypervisors are used, and what a VM sync may change."},
	{"export", "Export", "Where a saved export selection goes."},
	{"web", "Web UI", "Where this web UI listens and how it behaves."},
	{"paths", "Paths", "Where authama keeps its state, and the files it reads."},
}

// settingSectionsOf groups the settings in effect by section, each with its
// spec, in the page's order.
func settingSectionsOf(rows []SettingRow) []settingSection {
	by := map[string][]settingField{}
	for _, r := range rows {
		spec, ok := settings.Find(r.Key)
		if !ok {
			continue
		}
		by[spec.Section] = append(by[spec.Section], settingField{SettingRow: r, Spec: spec,
			InFile:     r.Source == "authama.json",
			Overridden: strings.HasPrefix(r.Source, "environment") || strings.HasPrefix(r.Source, ".env")})
	}
	var out []settingSection
	for _, s := range sectionTitles {
		if f := by[s.Key]; len(f) > 0 {
			out = append(out, settingSection{Key: s.Key, Title: s.Title, Summary: s.Summary, Fields: f})
		}
	}
	return out
}

// settingName is a setting's name for people: "age_recipient" reads "Age recipient".
func settingName(spec settings.Spec) string {
	n := strings.ReplaceAll(spec.Name, "_", " ")
	if n == "" {
		return n
	}
	return strings.ToUpper(n[:1]) + n[1:]
}
