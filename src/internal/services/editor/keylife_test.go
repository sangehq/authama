package editor

import (
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
)

func TestParseLifetime(t *testing.T) {
	for in, want := range map[string]int{"annual": 365, "Quarterly": 90, "monthly": 30, "120": 120, "1825": 1825} {
		if got, err := ParseLifetime(in); err != nil || got != want {
			t.Errorf("ParseLifetime(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "1826", "-5", "ten", "90d", "", "3.5"} {
		if _, err := ParseLifetime(in); err == nil {
			t.Errorf("ParseLifetime(%q) passed", in)
		}
	}
}

// A key's lifetime is set on its declaration (declared now if a host only
// named it) and its record, whose expiry is counted again from its creation;
// clearing it inherits the default; the default moves every key without one.
func TestSetKeyLifetimeRecountsTheExpiry(t *testing.T) {
	created := "2026-01-01"
	ed, p := writeManifest(t, twoHostsOneKeyJSON, map[string]inventory.KeyRecord{
		"SHA256:shared": {Profile: "work", Path: "~/.ssh/profiles/work/shared-ed25519", Type: "ed25519", Created: &created, RotateAfterDays: 365},
	})
	ref := manifest.KeyRef{Profile: "work", KeyName: "shared-ed25519"}

	res, err := ed.SetKeyLifetime(ref, 90)
	if err != nil {
		t.Fatal(err)
	}
	if res.Days != 90 || res.ExpiresOn != "2026-04-01" {
		t.Fatalf("90 days from 2026-01-01: %+v", res)
	}
	rec := loadInv(t, p).Keys["SHA256:shared"]
	if rec.RotateAfterDays != 90 || rec.ExpiresOn == nil || *rec.ExpiresOn != "2026-04-01" {
		t.Errorf("record = %d days, expires %v", rec.RotateAfterDays, rec.ExpiresOn)
	}
	m, _ := manifest.Load(p.Manifest())
	if m.RotateAfterDaysFor(ref) != 90 {
		t.Errorf("the declaration says %d", m.RotateAfterDaysFor(ref))
	}

	// The default does not move a key with a lifetime of its own...
	if _, err := ed.SetDefaultLifetime(180); err != nil {
		t.Fatal(err)
	}
	if got := *loadInv(t, p).Keys["SHA256:shared"].ExpiresOn; got != "2026-04-01" {
		t.Errorf("the default moved a key with its own lifetime: %s", got)
	}
	// ...and once it inherits, it follows the default.
	if res, err = ed.SetKeyLifetime(ref, 0); err != nil || res.Days != 180 || res.ExpiresOn != "2026-06-30" {
		t.Fatalf("cleared: %+v, %v; want 180 days, 2026-06-30", res, err)
	}
	if _, err := ed.SetKeyLifetime(ref, MaxLifetimeDays+1); err == nil {
		t.Error("a lifetime over the cap was accepted")
	}
	if _, err := ed.SetKeyLifetime(manifest.KeyRef{Profile: "work", KeyName: "nope"}, 30); err == nil {
		t.Error("a key the profile does not have was given a lifetime")
	}
}

// Revoking keeps the key and its hosts, drops the deployments it was removed
// from, keeps those it could not be, and records the fingerprint.
func TestRevokeKeyRecordsTheRevocation(t *testing.T) {
	when := "2026-01-01"
	ed, p := writeManifest(t, twoHostsOneKeyJSON, map[string]inventory.KeyRecord{
		"SHA256:shared": {
			Profile: "work", Path: "~/.ssh/profiles/work/shared-ed25519", Type: "ed25519",
			Deployments: []inventory.Deployment{{Target: "gh", Method: "manual", Date: &when, Verified: true}},
		},
	})
	ref := manifest.KeyRef{Profile: "work", KeyName: "shared-ed25519"}
	res, err := ed.RevokeKey(ref, "laptop lost")
	if err != nil {
		t.Fatal(err)
	}
	// No .pub on disk, so nothing can be matched remotely: reported, kept.
	if len(res.NotRemoved) != 1 || res.NotRemoved[0] != "gh" {
		t.Errorf("not removed = %v, want [gh]", res.NotRemoved)
	}
	if deps := loadInv(t, p).Keys["SHA256:shared"].Deployments; len(deps) != 1 {
		t.Errorf("a deployment it could not be removed from was dropped: %+v", deps)
	}
	revs, err := inventory.LoadRevocations(inventory.RevokedPath(p.Inventory()))
	if err != nil {
		t.Fatal(err)
	}
	r, ok := revs["SHA256:shared"]
	if !ok || r.Key != "work/shared-ed25519" || r.Reason != "laptop lost" || !strings.HasPrefix(r.RevokedOn, "20") {
		t.Fatalf("revocation = %+v", revs)
	}
	if m, _ := manifest.Load(p.Manifest()); len(m.Profiles["work"].Hosts) != 2 {
		t.Error("revoking removed hosts")
	}
}
