package change

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/importer"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// ErrImportWouldReplace is import refusing to replace a manifest that has
// profiles, which the CLI allows only with --force.
var ErrImportWouldReplace = errors.New("a non-empty manifest already exists - importing replaces it " +
	"rather than merging; re-run with --force, which backs the current " +
	"manifest and inventory up to <home>/.state/ first")

// SSHImport is `authama ssh import <source> [--force] --yes`: the ssh config
// at source read into a new manifest and inventory, replacing them.
//
// source is a path the caller chose, never one a person typed. Keys it names
// outside the managed tree are copied in; a preview's copy gets an empty file
// at each such path instead (importer.InPlaceOf), so no private key is
// copied to show it, and the preview still lists every key it would create.
func SSHImport(real paths.Paths, source string, replace bool) Change {
	args := []string{source}
	if replace {
		args = append(args, "--force")
	}
	args = append(args, "--yes")
	return Change{
		Command: "import", Args: args,
		Effects: func(p paths.Paths) ([]string, error) {
			if replace && hasProfiles(p) {
				return []string{"backs the current manifest and inventory up to " +
					filepath.Join(p.StateDir(), "import-backup-<time>") + " first"}, nil
			}
			return []string{}, nil
		},
		OnApply: func(p paths.Paths, _ io.Writer) error {
			if replace && hasProfiles(p) {
				importer.BackupTargets(p)
			}
			return nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if !replace && hasProfiles(p) {
				return ErrImportWouldReplace
			}
			res, err := importer.New(p, platform.EmitUseKeychain()).InPlaceOf(real).Run(source, false)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, res.Format())
			return nil
		},
	}
}

// hasProfiles says whether p's manifest holds a profile import would replace.
func hasProfiles(p paths.Paths) bool {
	m, err := manifest.Load(p.Manifest())
	return err == nil && len(m.Profiles) > 0
}
