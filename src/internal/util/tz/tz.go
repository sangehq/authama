// Package tz is the time zone authama shows times in (display.timezone,
// $AUTHAMA_TIMEZONE, or --tz for one command): this system's own unless one
// is set. Times are stored in UTC everywhere; only what is shown converts.
package tz

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	// The zone database is built in, so a zone resolves on a system without
	// one (Windows) the same as anywhere else.
	_ "time/tzdata"
)

var (
	mu   sync.RWMutex
	loc  = time.Local
	name = ""
)

// Load resolves a zone name: "" (or "local") is this system's zone.
func Load(zone string) (*time.Location, error) {
	zone = strings.TrimSpace(zone)
	if zone == "" || strings.EqualFold(zone, "local") {
		return time.Local, nil
	}
	if strings.HasPrefix(zone, "/") || strings.Contains(zone, "..") {
		return nil, fmt.Errorf("%q is not an IANA time zone name", zone)
	}
	l, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("%q is not an IANA time zone name (America/New_York, UTC)", zone)
	}
	return l, nil
}

// Set makes zone the one times are shown in; "" is this system's.
func Set(zone string) error {
	l, err := Load(zone)
	if err != nil {
		return err
	}
	mu.Lock()
	loc, name = l, strings.TrimSpace(zone)
	if strings.EqualFold(name, "local") {
		name = ""
	}
	mu.Unlock()
	return nil
}

// Location is the zone times are shown in.
func Location() *time.Location {
	mu.RLock()
	defer mu.RUnlock()
	return loc
}

// Name is the zone set, "" when it is this system's.
func Name() string {
	mu.RLock()
	defer mu.RUnlock()
	return name
}

// Describe names the zone in use: "America/New_York", or "this system's
// (EST)" when none is set.
func Describe() string {
	if n := Name(); n != "" {
		return n
	}
	abbr, _ := time.Now().In(time.Local).Zone()
	if time.Local.String() != "Local" {
		return "this system's (" + time.Local.String() + ", " + abbr + ")"
	}
	return "this system's (" + abbr + ")"
}

// In is t in the zone in use.
func In(t time.Time) time.Time { return t.In(Location()) }

// Format writes t in the zone in use. A zero time is "".
func Format(t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	return t.In(Location()).Format(layout)
}

// Minute is t to the minute, with the zone named: 2026-11-22 15:10 EST.
func Minute(t time.Time) string { return Format(t, "2006-01-02 15:04 MST") }

// Second is t to the second, with the zone named.
func Second(t time.Time) string { return Format(t, "2006-01-02 15:04:05 MST") }

// Search is the zones whose name contains q, any case; every zone for "".
func Search(q string) []string {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return append([]string(nil), Zones...)
	}
	var out []string
	for _, z := range Zones {
		if strings.Contains(strings.ToLower(z), q) {
			out = append(out, z)
		}
	}
	return out
}

var pinned atomic.Bool

// Pin is Set for a zone named for this run (--tz): it holds over the
// display.timezone setting for as long as the process runs.
func Pin(zone string) error {
	if err := Set(zone); err != nil {
		return err
	}
	pinned.Store(true)
	return nil
}

// Pinned reports whether this run's zone was named with Pin.
func Pinned() bool { return pinned.Load() }
