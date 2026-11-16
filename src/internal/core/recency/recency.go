// Package recency says how recent a key is: when each thing happened to it,
// how long ago that was, whether it is still "new", and the pill that sums it
// up. It is pure: the caller passes the facts, the policy and now.
//
// It is the one place a key event's age is worked out. The CLI, the web UI and
// --json all read its answer and compute nothing themselves
// (TestOnlyRecencyAgesAKeyEvent holds them to that).
package recency

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/expiry"
	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/util/envnames"
)

// Pill tones, mapped by the web UI to its badge-* classes.
const (
	ToneOK    = "ok"
	ToneInfo  = "info"
	ToneWarn  = "warn"
	ToneBad   = "bad"
	ToneMuted = "muted"
)

// DefaultWindowDays is how long a key stays new unless configured.
const DefaultWindowDays = 7

const day = 24 * time.Hour

// Eligible are the events that can make a key new.
var Eligible = []string{inventory.EventCreated, inventory.EventAdded, inventory.EventDeployed,
	inventory.EventRotated, inventory.EventRestored}

// How long a key stays new (keys.new_until).
const (
	// UntilWindow: new until the window ends, or an ack, whichever is first.
	UntilWindow = "window"
	// UntilAcknowledged: new until an ack, however long that takes.
	UntilAcknowledged = "acknowledged"
)

// Untils are the values keys.new_until takes, the default first.
var Untils = []string{UntilWindow, UntilAcknowledged}

// The default pill thresholds (keys.age_pills), as fractions of the key's
// lifetime (rotate_after_days): ageing from half of it, old from all of it.
const (
	DefaultWarn = 0.5
	DefaultBad  = 1.0
	// maxFraction bounds a threshold: ten lifetimes is already never.
	maxFraction = 10
)

// Policy is what counts as new and for how long, and when an age turns
// ageing (Warn) and old (Bad), as fractions of the key's lifetime.
type Policy struct {
	Window time.Duration // 0 turns the label off
	Events []string      // a subset of Eligible
	Until  string        // UntilWindow or UntilAcknowledged
	Warn   float64
	Bad    float64
}

// DefaultPolicy is seven days, after a key is created, added, rotated or
// deployed, or until an ack if sooner; ageing at half the key's lifetime and
// old at all of it.
func DefaultPolicy() Policy {
	return Policy{Window: DefaultWindowDays * day, Events: []string{inventory.EventCreated,
		inventory.EventAdded, inventory.EventRotated, inventory.EventDeployed},
		Until: UntilWindow, Warn: DefaultWarn, Bad: DefaultBad}
}

// ParsePills reads keys.age_pills: "warn,bad", two fractions of the key's
// lifetime, each above 0 and at most 10, warn no later than bad.
func ParsePills(s string) (warn, bad float64, err error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, errors.New("give two fractions of the key's lifetime, warn then bad, such as 0.5,1.0")
	}
	var f [2]float64
	for i, part := range parts {
		v, perr := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if perr != nil || v <= 0 || v > maxFraction {
			return 0, 0, fmt.Errorf("%q is not a fraction above 0 and at most %d", strings.TrimSpace(part), maxFraction)
		}
		f[i] = v
	}
	if f[0] > f[1] {
		return 0, 0, fmt.Errorf("warn (%g) must not come after bad (%g)", f[0], f[1])
	}
	return f[0], f[1], nil
}

// ParseEvents reads a comma-separated list of events, each one of Eligible.
func ParseEvents(s string) ([]string, error) {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !slices.Contains(Eligible, f) {
			return nil, fmt.Errorf("%q is not an event that can make a key new (use %s)", f, strings.Join(Eligible, ", "))
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil, errors.New("name at least one event")
	}
	return out, nil
}

// FromEnv is the policy the settings give. A value that does not parse leaves
// the default; `settings validate` names it.
func FromEnv(get envnames.Getenv) Policy {
	p := DefaultPolicy()
	if raw := strings.TrimSpace(envnames.Get(get, envnames.KeysNewWindowDays)); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			p.Window = time.Duration(n) * day
		}
	}
	if raw := envnames.Get(get, envnames.KeysNewEvents); raw != "" {
		if evs, err := ParseEvents(raw); err == nil {
			p.Events = evs
		}
	}
	if raw := strings.TrimSpace(envnames.Get(get, envnames.KeysNewUntil)); slices.Contains(Untils, raw) {
		p.Until = raw
	}
	if raw := envnames.Get(get, envnames.KeysAgePills); raw != "" {
		if w, b, err := ParsePills(raw); err == nil {
			p.Warn, p.Bad = w, b
		}
	}
	return p
}

// Event is one thing that happened to a key. DayOnly marks a time read from
// a day field: midnight, local time, of that day.
type Event struct {
	Kind    string
	At      time.Time
	DayOnly bool
}

// Timeline merges what key-events.json recorded with the inventory's day
// fields, oldest first. A precise time wins; a day field fills in for a record
// written before key-events.json existed. revokedOn and retiredOn are the
// days revoked.json and retired.json give, "" for none.
func Timeline(rec inventory.KeyRecord, ev inventory.KeyEvents, revokedOn, retiredOn string) []Event {
	days := map[string]string{
		inventory.EventCreated: deref(rec.Created),
		inventory.EventRevoked: revokedOn,
		inventory.EventRetired: retiredOn,
	}
	for _, d := range rec.Deployments {
		if v := deref(d.Date); v > days[inventory.EventDeployed] {
			days[inventory.EventDeployed] = v
		}
	}
	var out []Event
	for _, kind := range []string{inventory.EventCreated, inventory.EventAdded, inventory.EventDeployed,
		inventory.EventRotated, inventory.EventRestored, inventory.EventRetired, inventory.EventRevoked,
		inventory.EventAcknowledged} {
		if t, ok := ev.Time(kind); ok {
			out = append(out, Event{Kind: kind, At: t})
		} else if t, err := time.ParseInLocation(time.DateOnly, days[kind], time.Local); err == nil {
			out = append(out, Event{Kind: kind, At: t, DayOnly: true})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out
}

// Shown is one event as it is displayed: when (RFC 3339, or the date for a
// day-only one), how long ago, and its age against the key's constraints.
// Label is fresh, current, ageing or old; "" for an event that ends or
// acknowledges a key, which has no age to judge.
type Shown struct {
	Kind  string `json:"kind"`
	At    string `json:"at"`
	Ago   string `json:"ago"`
	Label string `json:"label,omitempty"`
	Tone  string `json:"tone"`
}

// Pill is the one word that sums a key up, with its tone.
type Pill struct {
	Label string `json:"label"`
	Tone  string `json:"tone"`
}

// Status is a key's recency. Age is how long ago the key was created, added
// or rotated in, whichever is latest; "" when none is known.
type Status struct {
	Age        string  `json:"age,omitempty"`
	Events     []Shown `json:"events"`
	IsNew      bool    `json:"is_new"`
	NewBecause string  `json:"new_because,omitempty"`
	Pill       Pill    `json:"pill"`
}

// Evaluate judges a timeline against the policy. lifetimeDays is the key's
// rotate_after_days, and expiryState its expiry engine state, which wins over
// age so the two never disagree.
func Evaluate(events []Event, lifetimeDays int, expiryState string, p Policy, now time.Time) Status {
	s := Status{Events: []Shown{}}
	lifetime := time.Duration(lifetimeDays) * day
	var ack, born, newAt time.Time
	last := map[string]time.Time{}
	for _, e := range events {
		last[e.Kind] = e.At
		switch e.Kind {
		case inventory.EventAcknowledged:
			ack = e.At
		case inventory.EventCreated, inventory.EventAdded, inventory.EventRotated:
			if born.IsZero() || e.At.After(born) {
				born = e.At
			}
		}
	}
	var bornAge time.Duration
	for _, e := range events {
		age := ageOf(e, now)
		sh := Shown{Kind: e.Kind, Ago: Ago(age), Tone: ToneMuted}
		if e.DayOnly {
			sh.At = e.At.Format(time.DateOnly)
			if age < day {
				sh.Ago = "today"
			}
		} else {
			sh.At = e.At.UTC().Format(time.RFC3339)
		}
		switch e.Kind {
		case inventory.EventRevoked, inventory.EventRetired, inventory.EventAcknowledged:
		default:
			sh.Label, sh.Tone = constraint(age, p, lifetime)
		}
		if e.At.Equal(born) {
			bornAge, s.Age = age, sh.Ago
		}
		s.Events = append(s.Events, sh)
		// The latest qualifying event names the reason; at a tie (a key minted
		// is created and added at once) the first, the more specific, stays.
		inWindow := age <= p.Window || p.Until == UntilAcknowledged
		if p.Window > 0 && slices.Contains(p.Events, e.Kind) && inWindow && e.At.After(ack) &&
			(!s.IsNew || e.At.After(newAt)) {
			s.IsNew, s.NewBecause, newAt = true, e.Kind, e.At
		}
	}
	retired := !last[inventory.EventRetired].IsZero() && !last[inventory.EventRestored].After(last[inventory.EventRetired])
	switch {
	case !last[inventory.EventRevoked].IsZero():
		s.Pill = Pill{"revoked", ToneMuted}
	case retired:
		s.Pill = Pill{"retired", ToneMuted}
	case expiryState == expiry.Overdue:
		s.Pill = Pill{"overdue", ToneBad}
	case expiryState == expiry.DueSoon:
		s.Pill = Pill{"due soon", ToneWarn}
	case s.IsNew:
		s.Pill = Pill{"new", ToneInfo}
	case born.IsZero():
		s.Pill = Pill{"unknown", ToneMuted}
	case lifetime > 0 && bornAge >= fraction(lifetime, p.Bad):
		s.Pill = Pill{"old", ToneBad}
	case lifetime > 0 && bornAge >= fraction(lifetime, p.Warn):
		s.Pill = Pill{"ageing", ToneWarn}
	default:
		s.Pill = Pill{"current", ToneOK}
	}
	return s
}

// constraint judges one event's age: fresh inside the new window, old past
// the policy's bad fraction of the key's lifetime, ageing past its warn
// fraction, current otherwise.
func constraint(age time.Duration, p Policy, lifetime time.Duration) (label, tone string) {
	switch {
	case p.Window > 0 && age <= p.Window:
		return "fresh", ToneInfo
	case lifetime > 0 && age >= fraction(lifetime, p.Bad):
		return "old", ToneBad
	case lifetime > 0 && age >= fraction(lifetime, p.Warn):
		return "ageing", ToneWarn
	}
	return "current", ToneOK
}

// fraction is f of the lifetime; a zero f (a Policy built by hand) is the
// default it stands for, so the zero value never makes every key old.
func fraction(lifetime time.Duration, f float64) time.Duration {
	if f <= 0 {
		return lifetime
	}
	return time.Duration(float64(lifetime) * f)
}

// ageOf is how long ago the event was. A day-only event is counted in whole
// calendar days, so a key created seven days ago is still seven days old all
// of today.
func ageOf(e Event, now time.Time) time.Duration {
	if !e.DayOnly {
		return now.Sub(e.At)
	}
	n := now.In(e.At.Location())
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, e.At.Location())
	return time.Duration(today.Sub(e.At).Hours()/24+0.5) * day
}

// Ago words a duration: "just now", "3 days ago", "in 2 months".
func Ago(d time.Duration) string {
	future := d < 0
	if future {
		d = -d
	}
	var n int
	var unit string
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n, unit = int(d/time.Minute), "minute"
	case d < 24*time.Hour:
		n, unit = int(d/time.Hour), "hour"
	case d < 30*24*time.Hour:
		n, unit = int(d/(24*time.Hour)), "day"
	case d < 365*24*time.Hour:
		n, unit = int(d/(30*24*time.Hour)), "month"
	default:
		n, unit = int(d/(365*24*time.Hour)), "year"
	}
	if n != 1 {
		unit += "s"
	}
	if future {
		return "in " + strconv.Itoa(n) + " " + unit
	}
	return strconv.Itoa(n) + " " + unit + " ago"
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
