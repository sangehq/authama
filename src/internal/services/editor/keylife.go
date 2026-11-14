package editor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/log"
)

// --- key lifetime ------------------------------------------------------------

// Lifetime bounds, in days. Five years is the longest any guidance gives a key
// used for authentication; a day is the shortest that leaves time to deploy.
const (
	MinLifetimeDays = 1
	MaxLifetimeDays = 5 * 365
)

// LifetimePresets are the lifetimes people are asked for by policy: a year is
// the common floor for SSH user keys, and stricter regimes ask for half, a
// quarter, or a month.
var LifetimePresets = map[string]int{"annual": 365, "semiannual": 180, "quarterly": 90, "monthly": 30}

// ParseLifetime reads a lifetime as a preset name or a number of days.
func ParseLifetime(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if d, ok := LifetimePresets[s]; ok {
		return d, nil
	}
	var d int
	if _, err := fmt.Sscanf(s, "%d", &d); err != nil || fmt.Sprint(d) != s {
		names := make([]string, 0, len(LifetimePresets))
		for n := range LifetimePresets {
			names = append(names, n)
		}
		sort.Strings(names)
		return 0, fmt.Errorf("lifetime %q is not a number of days or one of %s", s, strings.Join(names, ", "))
	}
	if d < MinLifetimeDays || d > MaxLifetimeDays {
		return 0, fmt.Errorf("lifetime %d days is outside %d to %d", d, MinLifetimeDays, MaxLifetimeDays)
	}
	return d, nil
}

// LifetimeResult is what SetKeyLifetime changed.
type LifetimeResult struct {
	Key       string
	Days      int
	ExpiresOn string // "" when the key has no creation date to count from
}

// SetKeyLifetime sets one key's lifetime: its declaration's rotate_after_days
// (declaring the key on its profile if only a host named it), and its
// inventory record's, with the expiry date counted again from when it was
// created. days 0 clears the key's own value, so it inherits the default.
func (e *Editor) SetKeyLifetime(ref manifest.KeyRef, days int) (LifetimeResult, error) {
	if days != 0 && (days < MinLifetimeDays || days > MaxLifetimeDays) {
		return LifetimeResult{}, fmt.Errorf("lifetime %d days is outside %d to %d", days, MinLifetimeDays, MaxLifetimeDays)
	}
	m, err := e.load()
	if err != nil {
		return LifetimeResult{}, err
	}
	p, ok := m.Profiles[ref.Profile]
	if !ok {
		return LifetimeResult{}, fmt.Errorf("unknown profile: %q", ref.Profile)
	}
	refs, err := m.KeyRefs()
	if err != nil {
		return LifetimeResult{}, err
	}
	known := false
	for _, r := range refs {
		known = known || r == ref
	}
	if !known {
		return LifetimeResult{}, fmt.Errorf("profile %q has no key %q", ref.Profile, ref.KeyName)
	}
	var set *int
	if days != 0 {
		set = &days
	}
	found := false
	for i := range p.Keys {
		if p.Keys[i].Name == ref.KeyName {
			p.Keys[i].RotateAfterDays, found = set, true
		}
	}
	if !found && set != nil {
		p.Keys = append(p.Keys, manifest.KeySpec{Name: ref.KeyName, RotateAfterDays: set})
	}
	m.SetProfile(ref.Profile, p)
	effective := m.RotateAfterDaysFor(ref)

	inv, err := inventory.Load(e.p.Inventory())
	if err != nil {
		return LifetimeResult{}, err
	}
	res := LifetimeResult{Key: ref.String(), Days: effective}
	ident := m.IdentityFile(ref.Profile, ref.KeyName)
	for fp, rec := range inv.Keys {
		if rec.Path != ident {
			continue
		}
		rec.RotateAfterDays = effective
		if rec.Created != nil && *rec.Created != "" {
			if exp, err := inventory.ComputeExpiry(*rec.Created, effective); err == nil {
				rec.ExpiresOn, res.ExpiresOn = &exp, exp
			}
		}
		inv.Keys[fp] = rec
	}
	if err := e.save(m); err != nil {
		return LifetimeResult{}, err
	}
	if err := inv.Save(e.p.Inventory()); err != nil {
		return LifetimeResult{}, err
	}
	log.Audit(e.p.AuditLog(), "key.lifetime", log.Field{Key: "profile", Value: ref.Profile},
		log.Field{Key: "key", Value: ref.KeyName}, log.Field{Key: "days", Value: effective})
	return res, nil
}

// --- revoke ------------------------------------------------------------------

// RevokeResult is what RevokeKey did.
type RevokeResult struct {
	Key         string
	Fingerprint string
	Removed     []string // targets the key was removed from
	NotRemoved  []string // targets it is deployed to but could not be removed from
}

// RevokeKey withdraws a key without deleting it: every target it is deployed
// to is asked, through its host's provider, to remove it; the deployments it
// was removed from are dropped from its record; and its fingerprint goes into
// revoked.json, so deploy refuses it from then on. The declaration, the hosts
// and the files stay: the next step is a new key, with `keygen KEY --force`
// or `rotate`, which gets a fingerprint of its own.
func (e *Editor) RevokeKey(ref manifest.KeyRef, reason string) (RevokeResult, error) {
	if err := e.p.RefuseOutsideSandbox("revoke"); err != nil {
		return RevokeResult{}, err
	}
	m, err := e.load()
	if err != nil {
		return RevokeResult{}, err
	}
	inv, err := inventory.Load(e.p.Inventory())
	if err != nil {
		return RevokeResult{}, err
	}
	ident := m.IdentityFile(ref.Profile, ref.KeyName)
	fp, rec, ok := "", inventory.KeyRecord{}, false
	for f, r := range inv.Keys {
		if r.Path == ident && !inventory.IsArchivedPathIn(r.Path, manifest.KeyRoot(m)) {
			fp, rec, ok = f, r, true
		}
	}
	if !ok {
		return RevokeResult{}, fmt.Errorf("key %s has no inventory record: it was never minted or reconciled, so there is nothing deployed to revoke", ref)
	}
	revs, err := inventory.LoadRevocations(inventory.RevokedPath(e.p.Inventory()))
	if err != nil {
		return RevokeResult{}, err
	}
	res := RevokeResult{Key: ref.String(), Fingerprint: fp}
	pub := m.KeyPath(e.p.SSHDir, ref) + ".pub"
	kept := rec.Deployments[:0:0]
	for _, d := range rec.Deployments {
		if h, found := hostByAlias(m, d.Target); found && e.removeFromTarget(h, pub) {
			res.Removed = append(res.Removed, d.Target)
			continue
		}
		res.NotRemoved = append(res.NotRemoved, d.Target)
		kept = append(kept, d)
	}
	rec.Deployments = kept
	inv.Keys[fp] = rec
	prev := revs[fp]
	revs[fp] = inventory.Revocation{
		Key: ref.String(), RevokedOn: inventory.Today(), Reason: reason,
		Removed: append(prev.Removed, res.Removed...), NotRemoved: res.NotRemoved,
	}
	inv.Note(fp, inventory.EventRevoked, inventory.Stamp())
	if err := inv.Save(e.p.Inventory()); err != nil {
		return res, err
	}
	if err := revs.Save(inventory.RevokedPath(e.p.Inventory())); err != nil {
		return res, err
	}
	log.Audit(e.p.AuditLog(), "key.revoke", log.Field{Key: "profile", Value: ref.Profile},
		log.Field{Key: "key", Value: ref.KeyName}, log.Field{Key: "fingerprint", Value: fp},
		log.Field{Key: "removed_from", Value: strings.Join(res.Removed, ",")},
		log.Field{Key: "not_removed_from", Value: strings.Join(res.NotRemoved, ",")})
	return res, nil
}

// SetDefaultLifetime sets defaults.rotate_after_days, the lifetime every key
// without one of its own inherits, and counts those keys' expiry dates again.
func (e *Editor) SetDefaultLifetime(days int) ([]LifetimeResult, error) {
	if days < MinLifetimeDays || days > MaxLifetimeDays {
		return nil, fmt.Errorf("lifetime %d days is outside %d to %d", days, MinLifetimeDays, MaxLifetimeDays)
	}
	m, err := e.load()
	if err != nil {
		return nil, err
	}
	m.Defaults.RotateAfterDays = days
	refs, err := m.KeyRefs()
	if err != nil {
		return nil, err
	}
	inv, err := inventory.Load(e.p.Inventory())
	if err != nil {
		return nil, err
	}
	var out []LifetimeResult
	for _, ref := range refs {
		if spec, ok := m.KeySpecFor(ref); ok && spec.RotateAfterDays != nil {
			continue // its own lifetime wins
		}
		ident := m.IdentityFile(ref.Profile, ref.KeyName)
		for fp, rec := range inv.Keys {
			if rec.Path != ident {
				continue
			}
			rec.RotateAfterDays = days
			r := LifetimeResult{Key: ref.String(), Days: days}
			if rec.Created != nil && *rec.Created != "" {
				if exp, err := inventory.ComputeExpiry(*rec.Created, days); err == nil {
					rec.ExpiresOn, r.ExpiresOn = &exp, exp
				}
			}
			inv.Keys[fp] = rec
			out = append(out, r)
		}
	}
	if err := e.save(m); err != nil {
		return nil, err
	}
	if err := inv.Save(e.p.Inventory()); err != nil {
		return nil, err
	}
	log.Audit(e.p.AuditLog(), "key.lifetime", log.Field{Key: "default", Value: true}, log.Field{Key: "days", Value: days})
	return out, nil
}
