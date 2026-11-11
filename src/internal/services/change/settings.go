package change

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/initsvc"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/homeperms"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// SettingsInit is `authama settings init`: write a starter authama.json (the
// schema reference and version, every setting at its default) into the
// config home. Without force it refuses a file that is already there; with
// force the file already there is kept beside the new one, as
// authama.json.before-reset-<time>, so resetting the settings to their
// defaults loses nothing. It touches nothing in ~/.ssh, so it takes no
// snapshot, as the command takes none.
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
			if old, err := os.ReadFile(path); err == nil {
				kept := path + ".before-reset-" + time.Now().Format("20060102-150405")
				if err := fs.WriteTextAtomic(kept, string(old), homeperms.FileMode); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(out, "kept the old settings as %s\n", kept)
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

// HomeInit is `authama init --force --backup`: the manifest, inventory and
// .env are written again with fresh defaults, after copying the old ones into
// the state home's .state/init-backup-<time>/. Profiles, hosts and token
// declarations go with the old manifest, so the form asks for the word
// "init" to be typed. ~/.ssh is not touched: keys and their files stay.
func HomeInit(confirmed bool) Change {
	return Change{
		Command: "init", Args: []string{"--force", "--backup"},
		Run: func(p paths.Paths, out io.Writer) error {
			if !confirmed {
				return errors.New(`type "init" to confirm`)
			}
			res, err := initsvc.New(p, platform.EmitUseKeychain()).Run(true, true, time.Now().Format("20060102-150405"))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(out, res.Format())
			return nil
		},
	}
}
