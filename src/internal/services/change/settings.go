package change

import (
	"fmt"
	"io"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/homeperms"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// SettingsInit is `authama settings init`: write a starter authama.json (the
// schema reference and version, every setting at its default) into the
// config home. Without force it refuses a file that is already there. It
// touches nothing in ~/.ssh, so it takes no snapshot, as the command takes
// none.
func SettingsInit(force bool) Change {
	var args []string
	if force {
		args = []string{"--force"}
	}
	return Change{
		Command: "settings init", Args: args, NoSnapshot: true,
		Run: func(p paths.Paths, out io.Writer) error {
			path := p.SettingsFile()
			if fs.Exists(path) && !force {
				return fmt.Errorf("%s already exists; --force replaces it", path)
			}
			if err := fs.EnsureDir(p.ConfigDir, homeperms.DirMode); err != nil {
				return err
			}
			if err := fs.WriteTextAtomic(path, string(settings.Starter()), homeperms.FileMode); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "wrote %s - add settings to it; `%s settings show --effective` lists them\n",
				path, brand.Command)
			return nil
		},
	}
}
