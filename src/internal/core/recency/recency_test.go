package recency

import (
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/expiry"
	"github.com/sangehq/authama/src/internal/core/inventory"
)

var now = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

func exact(kind string, ago time.Duration) Event { return Event{Kind: kind, At: now.Add(-ago)} }

func TestTheNewWindowHasExactEdges(t *testing.T) {
	p := DefaultPolicy()
	cases := []struct {
		name string
		ago  time.Duration
		want bool
	}{
		{"an hour old", time.Hour, true},
		{"exactly seven days", 7 * day, true},
		{"seven days and a second", 7*day + time.Second, false},
		{"a month", 30 * day, false},
	}
	for _, c := range cases {
		s := Evaluate([]Event{exact(inventory.EventCreated, c.ago)}, 365, expiry.OK, p, now)
		if s.IsNew != c.want {
			t.Errorf("%s: is_new %v, want %v", c.name, s.IsNew, c.want)
		}
	}
}

// Each event the policy names can make a key new, and new_because names the
// latest one; an event the policy leaves out cannot.
func TestOnlyThePolicysEventsMakeAKeyNew(t *testing.T) {
	old := exact(inventory.EventCreated, 200*day)
	deployed := exact(inventory.EventDeployed, 2*day)
	s := Evaluate([]Event{old, deployed}, 365, expiry.OK, DefaultPolicy(), now)
	if !s.IsNew || s.NewBecause != inventory.EventDeployed {
		t.Errorf("a recent deploy should make the key new: %+v", s)
	}
	p := DefaultPolicy()
	p.Events = []string{inventory.EventCreated}
	if s := Evaluate([]Event{old, deployed}, 365, expiry.OK, p, now); s.IsNew {
		t.Errorf("deployed is not in the policy, yet the key is new: %+v", s)
	}
	restored := exact(inventory.EventRestored, day)
	if s := Evaluate([]Event{old, restored}, 365, expiry.OK, DefaultPolicy(), now); s.IsNew {
		t.Error("restored is not a default event")
	}
}

func TestAWindowOfZeroTurnsTheLabelOff(t *testing.T) {
	p := DefaultPolicy()
	p.Window = 0
	if s := Evaluate([]Event{exact(inventory.EventCreated, time.Minute)}, 365, expiry.OK, p, now); s.IsNew {
		t.Error("window 0 must turn the label off")
	}
}

// An acknowledgement clears the label until a later event re-arms it.
func TestAnAcknowledgementClearsTheLabelUntilTheNextEvent(t *testing.T) {
	created := exact(inventory.EventCreated, 2*day)
	ack := exact(inventory.EventAcknowledged, day)
	if s := Evaluate([]Event{created, ack}, 365, expiry.OK, DefaultPolicy(), now); s.IsNew {
		t.Errorf("acknowledged key still new: %+v", s)
	}
	redeploy := exact(inventory.EventDeployed, time.Hour)
	if s := Evaluate([]Event{created, ack, redeploy}, 365, expiry.OK, DefaultPolicy(), now); !s.IsNew {
		t.Error("a deploy after the ack should make the key new again")
	}
}

// The main pill: an ended key is muted, expiry wins over age, new is info,
// and an older key is ok until half its lifetime, then ageing.
func TestThePillRulesApplyInOrder(t *testing.T) {
	cases := []struct {
		name   string
		events []Event
		state  string
		label  string
		tone   string
	}{
		{"revoked", []Event{exact(inventory.EventCreated, day), exact(inventory.EventRevoked, time.Hour)}, expiry.OK, "revoked", ToneMuted},
		{"retired", []Event{exact(inventory.EventCreated, 90*day), exact(inventory.EventRetired, time.Hour)}, expiry.OK, "retired", ToneMuted},
		{"restored after retiring", []Event{exact(inventory.EventCreated, 90*day), exact(inventory.EventRetired, 2*day), exact(inventory.EventRestored, day)}, expiry.OK, "current", ToneOK},
		{"overdue beats new", []Event{exact(inventory.EventCreated, day)}, expiry.Overdue, "overdue", ToneBad},
		{"due soon", []Event{exact(inventory.EventCreated, 340*day)}, expiry.DueSoon, "due soon", ToneWarn},
		{"new", []Event{exact(inventory.EventCreated, day)}, expiry.OK, "new", ToneInfo},
		{"current", []Event{exact(inventory.EventCreated, 100*day)}, expiry.OK, "current", ToneOK},
		{"ageing", []Event{exact(inventory.EventCreated, 200*day)}, expiry.OK, "ageing", ToneWarn},
		{"nothing known", nil, expiry.Unknown, "unknown", ToneMuted},
	}
	for _, c := range cases {
		s := Evaluate(c.events, 365, c.state, DefaultPolicy(), now)
		if s.Pill.Label != c.label || s.Pill.Tone != c.tone {
			t.Errorf("%s: pill %+v, want %s/%s", c.name, s.Pill, c.label, c.tone)
		}
	}
}

func TestEachEventCarriesItsAgeAndConstraint(t *testing.T) {
	s := Evaluate([]Event{
		exact(inventory.EventCreated, 300*day),
		exact(inventory.EventDeployed, 3*day),
		exact(inventory.EventRevoked, time.Hour),
	}, 365, expiry.OK, DefaultPolicy(), now)
	if len(s.Events) != 3 || s.Age != "10 months ago" {
		t.Fatalf("events: %+v, age %q", s.Events, s.Age)
	}
	want := []Shown{
		{Kind: inventory.EventCreated, Ago: "10 months ago", Label: "ageing", Tone: ToneWarn},
		{Kind: inventory.EventDeployed, Ago: "3 days ago", Label: "fresh", Tone: ToneInfo},
		{Kind: inventory.EventRevoked, Ago: "1 hour ago", Label: "", Tone: ToneMuted},
	}
	for i, w := range want {
		g := s.Events[i]
		if g.Kind != w.Kind || g.Ago != w.Ago || g.Label != w.Label || g.Tone != w.Tone {
			t.Errorf("event %d: got %+v, want %+v", i, g, w)
		}
		if g.At == "" {
			t.Errorf("event %d has no machine-readable time", i)
		}
	}
	old := Evaluate([]Event{exact(inventory.EventCreated, 400*day)}, 365, expiry.Overdue, DefaultPolicy(), now)
	if old.Events[0].Label != "old" || old.Events[0].Tone != ToneBad {
		t.Errorf("past its lifetime: %+v", old.Events[0])
	}
}

// Records written before key-events.json existed have only day fields: they
// still give a timeline, by the day, and the window is counted in days.
func TestDayOnlyLegacyRecordsStillGiveATimeline(t *testing.T) {
	created := "2026-11-07"
	deployed := "2026-11-08"
	rec := inventory.KeyRecord{Created: &created, Deployments: []inventory.Deployment{{Target: "a", Date: &deployed}}}
	evs := Timeline(rec, inventory.KeyEvents{}, "", "")
	if len(evs) != 2 || !evs[0].DayOnly || evs[0].Kind != inventory.EventCreated || evs[1].Kind != inventory.EventDeployed {
		t.Fatalf("timeline: %+v", evs)
	}
	local := time.Date(2026, 11, 14, 23, 0, 0, 0, time.Local)
	s := Evaluate(evs, 365, expiry.OK, DefaultPolicy(), local)
	if !s.IsNew || s.Events[0].Ago != "7 days ago" || s.Events[0].At != "2026-11-07" {
		t.Errorf("a key created seven calendar days ago is still new that day: %+v", s)
	}
	if s := Evaluate(evs[:1], 365, expiry.OK, DefaultPolicy(), local.Add(day)); s.IsNew {
		t.Error("eight calendar days is past the window")
	}
}

// A precise time wins over the day field; revoked_on and retired_on are read
// from their own records when key-events.json has nothing.
func TestTimelinePrefersThePreciseTime(t *testing.T) {
	created := "2026-11-01"
	ev := inventory.KeyEvents{At: map[string]string{inventory.EventCreated: "2026-11-01T08:15:00Z"}}
	evs := Timeline(inventory.KeyRecord{Created: &created}, ev, "2026-11-10", "")
	if len(evs) != 2 || evs[0].DayOnly || evs[0].At.Hour() != 8 || evs[1].Kind != inventory.EventRevoked || !evs[1].DayOnly {
		t.Errorf("timeline: %+v", evs)
	}
	if evs := Timeline(inventory.KeyRecord{}, inventory.KeyEvents{}, "", ""); len(evs) != 0 {
		t.Errorf("no facts, no events: %+v", evs)
	}
}

func TestParseEventsRefusesAnUnknownOrEmptyList(t *testing.T) {
	if got, err := ParseEvents(" created, deployed "); err != nil || strings.Join(got, ",") != "created,deployed" {
		t.Errorf("got %v %v", got, err)
	}
	for _, bad := range []string{"", "created,revoked", "made", ","} {
		if _, err := ParseEvents(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestFromEnvReadsTheSettingsAndFallsBackOnABadValue(t *testing.T) {
	env := map[string]string{"AUTHAMA_KEYS_NEW_WINDOW_DAYS": "3", "AUTHAMA_KEYS_NEW_EVENTS": "created"}
	p := FromEnv(func(k string) string { return env[k] })
	if p.Window != 3*day || strings.Join(p.Events, ",") != "created" {
		t.Errorf("got %+v", p)
	}
	env = map[string]string{"AUTHAMA_KEYS_NEW_WINDOW_DAYS": "soon", "AUTHAMA_KEYS_NEW_EVENTS": "nope"}
	if p := FromEnv(func(k string) string { return env[k] }); p.Window != 7*day || len(p.Events) != 4 {
		t.Errorf("a bad value must leave the default: %+v", p)
	}
}

func TestAgoWording(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "just now", time.Minute: "1 minute ago", 5 * time.Hour: "5 hours ago",
		day: "1 day ago", 45 * day: "1 month ago", 800 * day: "2 years ago", -2 * day: "in 2 days",
	} {
		if got := Ago(d); got != want {
			t.Errorf("Ago(%v) = %q, want %q", d, got, want)
		}
	}
}
