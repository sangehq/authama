package keysvc

import (
	"time"

	"github.com/sangehq/authama/src/internal/core/expiry"
	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/recency"
)

// Recency is how recent each row's key is, by its "profile/key" ref: the
// events recorded for it, how long ago each was, whether it is new under p,
// and its pill. inventoryPath locates revoked.json and retired.json, whose
// days stand in for a revocation or retirement key-events.json has no time
// for. A file that cannot be read is treated as empty: this is a display. A
// key never minted has no events and no pill.
func Recency(m *manifest.Manifest, inv *inventory.Inventory, inventoryPath string, rows []Row,
	p recency.Policy, now time.Time) map[string]recency.Status {
	revs, _ := inventory.LoadRevocations(inventory.RevokedPath(inventoryPath))
	rets, _ := inventory.LoadRetirements(inventory.RetiredPath(inventoryPath))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	states := map[string]string{}
	for _, st := range expiry.ComputeStatesIn(inv, m.Defaults.WarnBeforeDays, today, manifest.KeyRoot(m)) {
		states[st.Fingerprint] = st.State
	}
	out := make(map[string]recency.Status, len(rows))
	for _, r := range rows {
		rec, ok := inv.Keys[r.Fingerprint]
		if !ok || r.Fingerprint == "" {
			out[r.Ref.String()] = recency.Status{Events: []recency.Shown{}} // never minted: nothing to judge
			continue
		}
		events := recency.Timeline(rec, inv.Events[r.Fingerprint], revs[r.Fingerprint].RevokedOn, rets[r.Fingerprint].RetiredOn)
		state := states[r.Fingerprint]
		if state == "" {
			state = expiry.Unknown
		}
		out[r.Ref.String()] = recency.Evaluate(events, r.RotateAfterDays, state, p, now)
	}
	return out
}
