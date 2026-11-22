package reconciler

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// PredecessorStillLogsIn names the hosts the key now in old/ was never removed
// from, as its retirement recorded them. The archive keeps one predecessor,
// so rotating again would evict it while those hosts still accept it (review
// R-BUG-12). A key in old/ with no retirement on record names none only when
// its inventory record shows it was never deployed; otherwise - deployed, or
// archived before records were kept and unknown to the inventory - it is
// "cannot tell", as is a key whose fingerprint cannot be read: an error, which
// callers treat as still logging in unless --evict-old (review A-BUG-07).
// Local replacements record a retirement too (Reconciler.archiveRecord).
func PredecessorStillLogsIn(p paths.Paths, m *manifest.Manifest, ks *keystore.KeyStore, keyRef manifest.KeyRef) ([]string, error) {
	slot := filepath.Join(manifest.ArchiveDir(m, p.SSHDir, keyRef), keyRef.KeyName)
	if _, err := os.Stat(slot + ".pub"); err != nil {
		return nil, nil
	}
	fp, err := ks.Fingerprint(slot + ".pub")
	if err != nil {
		return nil, fmt.Errorf("cannot read the fingerprint of %s.pub: %w", filepath.Base(slot), err)
	}
	all, err := inventory.LoadRetirements(inventory.RetiredPath(p.Inventory()))
	if err != nil {
		// Unreadable is not "nothing recorded": stop rather than evict.
		return nil, err
	}
	if rec, ok := all[fp]; ok {
		return rec.NotRemoved, nil
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if inv != nil {
		if rec, ok := inv.Keys[fp]; ok && len(rec.Deployments) == 0 {
			return nil, nil // never deployed: it logs in nowhere
		}
	}
	return nil, fmt.Errorf("the previous key in old/ (%s) has no retirement record, so where it was deployed and "+
		"never removed is unknown", fp)
}
