package change

import (
	"fmt"
	"io"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/selection"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ExportSelectionSave is `authama export selections save <name> --to
// manifest`: keep sel in the manifest's defaults.export.selections, so it
// travels with the manifest. This account's own store is not a change: the
// web saves there directly, as the CLI does.
func ExportSelectionSave(name string, sel selection.Selection) Change {
	return Change{
		Command: "export selections save", Args: []string{name, "--to", savedsel.StoreManifest, "--yes"},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := savedsel.Save(p, savedsel.StoreManifest, name, sel); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "saved %s (%s rows) in the manifest; export it with `%s export %s`\n", name, sel.Grain, brand.Command, name)
			return nil
		},
	}
}

// ExportSelectionDelete is `authama export selections delete <name> --from
// manifest`.
func ExportSelectionDelete(name string) Change {
	return Change{
		Command: "export selections delete", Args: []string{name, "--from", savedsel.StoreManifest, "--yes"},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := savedsel.Delete(p, savedsel.StoreManifest, name); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "deleted %s from the manifest\n", name)
			return nil
		},
	}
}
