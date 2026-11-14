package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/util/paths"
)

type listedKey struct {
	Key     string `json:"key"`
	Recency struct {
		Age        string `json:"age"`
		IsNew      bool   `json:"is_new"`
		NewBecause string `json:"new_because"`
		Pill       struct{ Label, Tone string }
		Events     []struct{ Kind, At, Ago string }
	} `json:"recency"`
}

func listedKeys(t *testing.T, args ...string) []listedKey {
	t.Helper()
	out := run(t, append([]string{"ssh", "key", "list", "--json"}, args...)...)
	if strings.Contains(out, "\x1b") {
		t.Errorf("--json carries a terminal escape:\n%q", out)
	}
	var keys []listedKey
	if err := json.Unmarshal([]byte(out), &keys); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return keys
}

func recencyHome(t *testing.T) paths.Paths {
	t.Helper()
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "host", "add", "work", "db", "-H", "db.example", "-u", "ops")
	run(t, "ssh", "reconcile", "--yes", "--no-pin")
	return p
}

// A key minted now is new, because it was created; the list says so in the
// table, in --json, and --new keeps only such keys. Minting recorded the
// time in key-events.json, in the same save as the inventory record.
func TestAMintedKeyIsListedAsNew(t *testing.T) {
	p := recencyHome(t)
	keys := listedKeys(t)
	if len(keys) != 2 {
		t.Fatalf("listed %d keys, want 2", len(keys))
	}
	for _, k := range keys {
		r := k.Recency
		if !r.IsNew || r.NewBecause != inventory.EventCreated || r.Pill.Label != "new" || r.Pill.Tone != "info" || r.Age != "just now" {
			t.Errorf("%s: %+v", k.Key, r)
		}
		if len(r.Events) < 2 || r.Events[0].At == "" {
			t.Errorf("%s: events %+v", k.Key, r.Events)
		}
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		t.Fatal(err)
	}
	for fp := range inv.Keys {
		if ev := inv.Events[fp]; ev.Origin != inventory.OriginGenerated {
			t.Errorf("%s: no generated origin recorded: %+v", fp, ev)
		}
	}
	table := run(t, "ssh", "key", "list")
	if !strings.Contains(table, "AGE") || !strings.Contains(table, "STATUS") || !strings.Contains(table, "just now") || !strings.Contains(table, "new") {
		t.Errorf("the table has no age or status:\n%s", table)
	}
	if strings.Contains(table, "\x1b") {
		t.Error("the table carries a terminal escape")
	}
	if n := len(listedKeys(t, "--new")); n != 2 {
		t.Errorf("--new kept %d keys, want 2", n)
	}
}

// `key ack` clears the label on the keys it names, and --all on every key;
// a key not named keeps it.
func TestAckClearsTheNewLabel(t *testing.T) {
	recencyHome(t)
	keys := listedKeys(t)
	out := run(t, "ssh", "key", "ack", keys[0].Key)
	if !strings.Contains(out, keys[0].Key) {
		t.Errorf("ack did not name the key:\n%s", out)
	}
	for _, k := range listedKeys(t) {
		if want := k.Key != keys[0].Key; k.Recency.IsNew != want {
			t.Errorf("%s: is_new %v, want %v", k.Key, k.Recency.IsNew, want)
		}
	}
	if n := len(listedKeys(t, "--new")); n != 1 {
		t.Errorf("--new kept %d keys after one ack, want 1", n)
	}
	run(t, "ssh", "key", "ack", "--all")
	if n := len(listedKeys(t, "--new")); n != 0 {
		t.Errorf("--new kept %d keys after ack --all, want 0", n)
	}
	if _, err := runIn(t, "", "ssh", "key", "ack"); err == nil {
		t.Error("ack with nothing named must refuse")
	}
}

// keys.new_window_days 0 turns the label off without losing the ages.
func TestAWindowOfZeroDaysTurnsTheLabelOff(t *testing.T) {
	recencyHome(t)
	t.Setenv("AUTHAMA_KEYS_NEW_WINDOW_DAYS", "0")
	for _, k := range listedKeys(t) {
		if k.Recency.IsNew || k.Recency.Age == "" {
			t.Errorf("%s: %+v", k.Key, k.Recency)
		}
	}
}

// Revoking records the time too, and the key's pill becomes revoked.
func TestARevokedKeyIsListedAsRevoked(t *testing.T) {
	p := recencyHome(t)
	keys := listedKeys(t)
	run(t, "ssh", "key", "revoke", keys[0].Key, "--yes")
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		t.Fatal(err)
	}
	revoked := 0
	for _, ev := range inv.Events {
		if _, ok := ev.Time(inventory.EventRevoked); ok {
			revoked++
		}
	}
	if revoked != 1 {
		t.Errorf("%d keys carry a revoked time, want 1", revoked)
	}
	if got := listedKeys(t, keys[0].Key)[0].Recency.Pill.Label; got != "revoked" {
		t.Errorf("pill %q, want revoked", got)
	}
	if _, err := os.Stat(inventory.EventsPath(p.Inventory())); err != nil {
		t.Error(err)
	}
}
