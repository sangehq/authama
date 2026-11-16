package agent

import (
	"reflect"
	"testing"
)

// LoadReport names what it could not load, so a caller can say so: a key file
// that is not on disk, and a key the agent refused (audit B-03).
func TestLoadReportNamesMissingAndFailedKeys(t *testing.T) {
	m := loadManifest(t)
	ssh := t.TempDir()
	gh, box := keyName(t, m, "work", "gh"), keyName(t, m, "work", "box")
	touch(t, ssh, "work", gh)

	r, err := LoadReport(m, ssh, "work", func(string) bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Added) != 0 || !reflect.DeepEqual(r.Failed, []string{gh}) || !reflect.DeepEqual(r.Missing, []string{box}) {
		t.Errorf("report = %+v, want failed [%s] and missing [%s]", r, gh, box)
	}

	r, _ = LoadReport(m, ssh, "work", func(string) bool { return true })
	if !reflect.DeepEqual(r.Added, []string{gh}) || len(r.Failed) != 0 || !reflect.DeepEqual(r.Missing, []string{box}) {
		t.Errorf("report = %+v, want added [%s] and missing [%s]", r, gh, box)
	}
}
