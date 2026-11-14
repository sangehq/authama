package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sangehq/authama/src/internal/util/fs"
)

// The events a key goes through, as key-events.json records them.
const (
	EventCreated      = "created"      // minted by authama, or by a rotation
	EventAdded        = "added"        // recorded: minted, adopted or imported
	EventDeployed     = "deployed"     // installed on a target (the latest one)
	EventRotated      = "rotated"      // the successor a rotation minted
	EventRestored     = "restored"     // brought back by a rollback
	EventRetired      = "retired"      // replaced by a rotation, or rolled back
	EventRevoked      = "revoked"      // taken off its targets for good
	EventAcknowledged = "acknowledged" // `ssh key ack`: no longer new to you
)

// Where a key came from, for EventAdded.
const (
	OriginGenerated = "generated"
	OriginImported  = "imported"
	OriginAdopted   = "adopted"
)

// KeyEvents is when each event last happened to one key, to the second, and
// where the key came from. The inventory's day fields (created, a
// deployment's date, revoked_on, retired_on) stay authoritative for what they
// say; these add the time of day and the events they never held.
type KeyEvents struct {
	Origin      string            `json:"origin,omitempty"`
	RotatedFrom string            `json:"rotated_from,omitempty"`
	At          map[string]string `json:"at"` // event -> RFC 3339, UTC
}

// Time is when the event last happened, if it was recorded.
func (e KeyEvents) Time(event string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, e.At[event])
	return t, err == nil
}

// EventsPath is key-events.json, beside the inventory at inventoryPath. A file
// of its own for the reason revoked.json is one: the inventory decodes
// strictly, so a new field would make an older authama refuse it.
func EventsPath(inventoryPath string) string {
	return filepath.Join(filepath.Dir(inventoryPath), "key-events.json")
}

// Stamp is the time a writer records an event at: now, in UTC, to the second.
// It is the one clock the writers read, as Today is for the day fields.
func Stamp() time.Time { return time.Now().UTC().Truncate(time.Second) }

// Note records that event happened to the key at at, replacing an earlier
// time for the same event. Save writes it with the inventory.
func (inv *Inventory) Note(fingerprint, event string, at time.Time) {
	if inv.Events == nil {
		inv.Events = map[string]KeyEvents{}
	}
	e := inv.Events[fingerprint]
	if e.At == nil {
		e.At = map[string]string{}
	}
	e.At[event] = at.UTC().Format(time.RFC3339)
	inv.Events[fingerprint] = e
}

// NoteAdded records a key entering the inventory and where it came from. A
// generated key is created then as well.
func (inv *Inventory) NoteAdded(fingerprint, origin string, at time.Time) {
	inv.Note(fingerprint, EventAdded, at)
	if origin == OriginGenerated {
		inv.Note(fingerprint, EventCreated, at)
	}
	e := inv.Events[fingerprint]
	e.Origin = origin
	inv.Events[fingerprint] = e
}

// NoteRotated records the successor a rotation minted, and the key it replaced.
func (inv *Inventory) NoteRotated(fingerprint, from string, at time.Time) {
	inv.NoteAdded(fingerprint, OriginGenerated, at)
	inv.Note(fingerprint, EventRotated, at)
	e := inv.Events[fingerprint]
	e.RotatedFrom = from
	inv.Events[fingerprint] = e
}

// loadEvents reads key-events.json; a missing file is no events.
func loadEvents(path string) (map[string]KeyEvents, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]KeyEvents{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("key events could not be read: %s: %w", path, err)
	}
	var ev map[string]KeyEvents
	if err := json.Unmarshal(b, &ev); err != nil {
		return nil, fmt.Errorf("key events failed validation: %s: %w", path, err)
	}
	if ev == nil {
		ev = map[string]KeyEvents{}
	}
	return ev, nil
}

// saveEvents writes key-events.json owner-only. With no events it writes
// nothing, so a home that never recorded one gains no file.
func saveEvents(path string, ev map[string]KeyEvents) error {
	if len(ev) == 0 {
		return nil
	}
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteTextAtomic(path, string(b)+"\n", 0o600)
}
