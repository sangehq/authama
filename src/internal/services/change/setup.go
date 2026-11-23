package change

import (
	"fmt"
	"io"
	"os"

	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// SetupImport is the config half of `authama setup import`: the setup
// document merged into the home under policy, the inventory too for a full
// export (nil otherwise), and ~/.ssh/config re-rendered when the manifest
// changed. The key files and token values of a full export are laid down by
// the CLI around it; the web imports documents only.
func SetupImport(d setupxfer.Doc, inventory []byte, policy setupxfer.Policy) Change {
	return Change{
		Command: "setup import", Args: []string{"--on-conflict=" + string(policy), "--yes"},
		Run: func(p paths.Paths, out io.Writer) error {
			res, err := setupxfer.Merge(p, d, policy)
			if err != nil {
				return err
			}
			inv, err := setupxfer.MergeInventory(p, inventory, policy)
			if err != nil {
				return err
			}
			res.Items = append(res.Items, inv.Items...)
			_, _ = fmt.Fprint(out, res.Format())
			c := res.Counts()
			if c[setupxfer.StateAdd]+c[setupxfer.StateReplace] == 0 {
				return nil
			}
			log.Audit(p.AuditLog(), "setup.import", log.Field{Key: "added", Value: c[setupxfer.StateAdd]},
				log.Field{Key: "replaced", Value: c[setupxfer.StateReplace]}, log.Field{Key: "kept", Value: c[setupxfer.StateKept]},
				log.Field{Key: "on_conflict", Value: string(policy)})
			if _, err := os.Stat(p.Manifest()); err == nil {
				return ApplyManifestEdit(out, p)
			}
			return nil
		},
	}
}
