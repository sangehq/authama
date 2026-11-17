package reconciler

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/providers"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Eviction is what evicting a key's previous key did: the hosts it was taken
// off, and those it still logs in to because it could not be taken off there
// (a target that takes keys by hand, a host that did not answer, or one no
// longer in the manifest), where it has to be removed by hand.
type Eviction struct {
	Removed, Still []string
}

// EvictPredecessor takes the key in ref's old/ slot - the previous key - off
// every host its retirement record says it was never removed from, before a
// rotation or a forced replacement drops it from old/. It is the evict half
// of the previous-key policy (rotate --evict-old, and --force unless
// --keep-old): a factory reset should leave only the new key. The record is
// updated, so the key's checklist names only the hosts it is still on. With
// no key in old/, or none recorded as still trusted, it does nothing.
func EvictPredecessor(p paths.Paths, m *manifest.Manifest, ks *keystore.KeyStore, ref manifest.KeyRef) (Eviction, error) {
	slot := filepath.Join(manifest.ArchiveDir(m, p.SSHDir, ref), ref.KeyName)
	pub, err := os.ReadFile(slot + ".pub")
	if err != nil {
		return Eviction{}, nil // nothing in old/
	}
	fp, err := ks.Fingerprint(slot + ".pub")
	if err != nil {
		return Eviction{}, nil // no fingerprint, no retirement to look up
	}
	path := inventory.RetiredPath(p.Inventory())
	all, err := inventory.LoadRetirements(path)
	if err != nil {
		return Eviction{}, err
	}
	rec, ok := all[fp]
	if !ok || len(rec.NotRemoved) == 0 {
		return Eviction{}, nil
	}
	hosts, err := m.HostsForKey(ref)
	if err != nil {
		return Eviction{}, err
	}
	byAlias := map[string]manifest.Host{}
	for _, h := range hosts {
		byAlias[h.Alias] = h
	}
	var ev Eviction
	for _, alias := range rec.NotRemoved {
		h, ok := byAlias[alias]
		if !ok {
			ev.Still = append(ev.Still, alias)
			continue
		}
		prov := providers.Resolve(deref(h.Provider), p.Providers())
		tgt := providers.Target{Alias: h.Alias, Hostname: h.Hostname, User: h.User, Port: h.Port,
			PubkeyPath: slot + ".pub", PubkeyText: string(pub), TokenEnv: deref(h.TokenEnv),
			KnownHosts: filepath.Join(p.SSHDir, "known_hosts")}
		if prov.Remove(tgt) {
			ev.Removed = append(ev.Removed, alias)
			continue
		}
		ev.Still = append(ev.Still, alias)
	}
	rec.Removed = append(rec.Removed, ev.Removed...)
	rec.NotRemoved = slices.Clone(ev.Still)
	for _, a := range ev.Removed {
		delete(rec.ByHand, a)
	}
	all[fp] = rec
	if err := all.Save(path); err != nil {
		return ev, fmt.Errorf("saving %s: %w", path, err)
	}
	return ev, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
