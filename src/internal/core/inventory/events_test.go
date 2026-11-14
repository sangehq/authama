package inventory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var at = time.Date(2026, 11, 14, 9, 30, 0, 0, time.UTC)

// Events ride on the inventory: what a writer notes is saved by the same
// Save, beside it, and read back by Load.
func TestNotedEventsAreSavedAndLoadedWithTheInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	inv := New()
	inv.Record("fp-new", KeyRecord{Profile: "work", Path: "~/.ssh/profiles/work/gh"})
	inv.NoteAdded("fp-new", OriginGenerated, at)
	inv.Note("fp-new", EventDeployed, at.Add(time.Hour))
	inv.NoteRotated("fp-succ", "fp-old", at.Add(2*time.Hour))
	inv.Note("fp-old", EventRetired, at.Add(2*time.Hour))
	if err := inv.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ev := got.Events["fp-new"]
	if ev.Origin != OriginGenerated || ev.RotatedFrom != "" {
		t.Errorf("origin not kept: %+v", ev)
	}
	for kind, want := range map[string]time.Time{EventCreated: at, EventAdded: at, EventDeployed: at.Add(time.Hour)} {
		if g, ok := ev.Time(kind); !ok || !g.Equal(want) {
			t.Errorf("%s: got %v %v, want %v", kind, g, ok, want)
		}
	}
	succ := got.Events["fp-succ"]
	if succ.RotatedFrom != "fp-old" || succ.Origin != OriginGenerated {
		t.Errorf("rotation not kept: %+v", succ)
	}
	for _, kind := range []string{EventCreated, EventAdded, EventRotated} {
		if g, ok := succ.Time(kind); !ok || !g.Equal(at.Add(2*time.Hour)) {
			t.Errorf("successor %s: got %v %v", kind, g, ok)
		}
	}
	if _, ok := got.Events["fp-old"].Time(EventRetired); !ok {
		t.Error("the predecessor's retirement was not kept")
	}
	// The inventory itself is unchanged in shape: no events inside it, so an
	// older authama, which decodes it strictly, still reads it.
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "deployed_at") || strings.Contains(string(b), EventRotated) {
		t.Errorf("events leaked into inventory.json:\n%s", b)
	}
	if fi, err := os.Stat(EventsPath(path)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("key-events.json not written owner-only: %v %v", fi, err)
	}
}

// Old state still loads: an inventory with no events file beside it has no
// events, and saving it writes none, so nothing appears that was not there.
func TestAnInventoryWithNoEventsFileLoadsAndSavesAsBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"keys":{"fp":{"profile":"p","path":"x","created":"2026-01-02"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inv, err := Load(path)
	if err != nil || len(inv.Events) != 0 {
		t.Fatalf("old inventory: %v %v", inv.Events, err)
	}
	if err := inv.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(EventsPath(path)); !os.IsNotExist(err) {
		t.Errorf("saving an inventory with no events wrote %s", EventsPath(path))
	}
}

func TestAnUnreadableEventsFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.json")
	if err := New().Save(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(EventsPath(path), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "key-events.json") {
		t.Errorf("a corrupt key-events.json must be named, got %v", err)
	}
}

// A later note of the same event replaces the earlier one: a redeploy
// re-arms "deployed".
func TestALaterNoteReplacesTheEarlierOne(t *testing.T) {
	inv := New()
	inv.Note("fp", EventDeployed, at)
	inv.Note("fp", EventDeployed, at.Add(48*time.Hour))
	if g, _ := inv.Events["fp"].Time(EventDeployed); !g.Equal(at.Add(48 * time.Hour)) {
		t.Errorf("got %v", g)
	}
}
