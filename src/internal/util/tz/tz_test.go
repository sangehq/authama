package tz

import (
	"strings"
	"testing"
	"time"
)

func TestAZoneIsCheckedAndApplied(t *testing.T) {
	t.Cleanup(func() { _ = Set("") })
	if err := Set("Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	if got := Minute(at); got != "2026-07-01 12:00 CEST" {
		t.Fatalf("Minute = %q", got)
	}
	for _, bad := range []string{"Nowhere/Place", "../etc/passwd", "/etc/localtime"} {
		if err := Set(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if Name() != "Europe/Berlin" {
		t.Fatal("a refused zone replaced the one in use")
	}
	if err := Set("local"); err != nil || Name() != "" || Location() != time.Local {
		t.Fatalf("local: %v %q", err, Name())
	}
}

func TestZonesAreSearchable(t *testing.T) {
	if len(Zones) < 300 || Zones[0] == "" {
		t.Fatalf("%d zones", len(Zones))
	}
	got := Search("berl")
	if len(got) != 1 || got[0] != "Europe/Berlin" {
		t.Fatalf("Search(berl) = %v", got)
	}
	for _, z := range Search("new_y") {
		if !strings.Contains(strings.ToLower(z), "new_y") {
			t.Fatalf("Search returned %q", z)
		}
	}
	for _, z := range Zones {
		if _, err := time.LoadLocation(z); err != nil {
			t.Fatalf("%s: %v", z, err)
		}
	}
}
