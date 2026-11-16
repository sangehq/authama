package reconciler

import (
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
// R-BUG-12). A key in old/ with no retirement on record names none.
func PredecessorStillLogsIn(p paths.Paths, m *manifest.Manifest, ks *keystore.KeyStore, keyRef manifest.KeyRef) ([]string, error) {
	slot := filepath.Join(manifest.ArchiveDir(m, p.SSHDir, keyRef), keyRef.KeyName)
	if _, err := os.Stat(slot + ".pub"); err != nil {
		return nil, nil
	}
	fp, err := ks.Fingerprint(slot + ".pub")
	if err != nil {
		return nil, nil // no fingerprint, no retirement to look up
	}
	all, err := inventory.LoadRetirements(inventory.RetiredPath(p.Inventory()))
	if err != nil {
		// Unreadable is not "nothing recorded": stop rather than evict.
		return nil, err
	}
	return all[fp].NotRemoved, nil
}
