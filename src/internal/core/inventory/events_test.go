package inventory

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/fs"
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
	// Windows has no owner-only permission bits, so the mode is checked elsewhere.
	if fi, err := os.Stat(EventsPath(path)); err != nil {
		t.Errorf("key-events.json not written: %v", err)
	} else if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("key-events.json not written owner-only: %v", fi.Mode().Perm())
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

// A save interrupted between its writes leaves inventory.json and
// key-events.json agreeing: a crash before the inventory is written keeps the
// old pair, and one after it keeps the new pair, read from the journal.
func TestAnInterruptedSaveLeavesThePairConsistent(t *testing.T) {
	cases := []struct {
		name, failOn string
		wantNew      bool
	}{
		{"the journal is not written", "key-events.json.pending", false},
		{"the inventory is not written", "inventory.json", false},
		{"the events are not written", "key-events.json", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inventory.json")
			inv := New()
			inv.Record("fp-old", KeyRecord{Profile: "work", Path: "~/.ssh/profiles/work/old"})
			inv.NoteAdded("fp-old", OriginGenerated, at)
			if err := inv.Save(path); err != nil {
				t.Fatal(err)
			}
			inv.Record("fp-new", KeyRecord{Profile: "work", Path: "~/.ssh/profiles/work/new"})
			inv.NoteAdded("fp-new", OriginGenerated, at.Add(time.Hour))
			inv.Note("fp-old", EventRetired, at.Add(time.Hour))
			writeAtomic = func(p, text string, mode os.FileMode) error {
				if filepath.Base(p) == c.failOn {
					return errors.New("interrupted")
				}
				return fs.WriteTextAtomic(p, text, mode)
			}
			err := inv.Save(path)
			writeAtomic = fs.WriteTextAtomic
			if err == nil {
				t.Fatal("the interrupted save reported success")
			}
			got, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			_, key := got.Keys["fp-new"]
			_, added := got.Events["fp-new"]
			_, retired := got.Events["fp-old"].Time(EventRetired)
			if key != c.wantNew || added != c.wantNew || retired != c.wantNew {
				t.Errorf("inventory and events disagree: new key recorded %v, its events %v, the old key's retirement %v; want all %v",
					key, added, retired, c.wantNew)
			}
			if _, ok := got.Events["fp-old"].Time(EventCreated); !ok {
				t.Error("the events saved before the interruption were lost")
			}
			// The next save completes normally and leaves no journal behind.
			if err := got.Save(path); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(journalPath(path)); !os.IsNotExist(err) {
				t.Errorf("a completed save left %s: %v", journalPath(path), err)
			}
		})
	}
}
