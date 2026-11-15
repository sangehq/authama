package change

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/editor"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// KeyRoot is `authama ssh key root <dir> --yes`: ~/.ssh/<dir>/ made the key
// root, and the whole tree of profile directories moved there in one rename.
//
// It moves every key on disk, so it is made only when confirm repeats the
// new name exactly. A preview's copy holds a stand-in for each private key,
// so the preview lists every key it would move without holding one.
func KeyRoot(target, confirm string) Change {
	if target == "" {
		target = manifest.DefaultKeyRoot
	}
	return Change{
		Command: "key root", Args: []string{target, "--yes"},
		Effects: func(p paths.Paths) ([]string, error) {
			if err := manifest.CheckKeyRoot(target); err != nil {
				return nil, err
			}
			current := manifest.KeyRootAt(p.Manifest())
			if target == current {
				return nil, fmt.Errorf("the key root is already ~/.ssh/%s/", current)
			}
			if confirm != target {
				return nil, errors.New("type the new directory name again to confirm: it moves every key")
			}
			return []string{
				fmt.Sprintf("moves ~/.ssh/%s/ to ~/.ssh/%s/ in one rename: private keys, .pub files, old/ archives and category config files", current, target),
				"keys adopted where they are stay put",
			}, nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			res, err := editor.MoveKeyRoot(editor.New(p), target)
			if err != nil {
				return err
			}
			verb := "keys will be minted under"
			if res.Moved {
				verb = "moved the keys to"
			}
			_, _ = fmt.Fprintf(out, "✓ %s %s (was %s); %d inventory record(s) updated\n",
				verb, filepath.ToSlash(filepath.Join("~/.ssh", res.To))+"/", "~/.ssh/"+res.From+"/", res.RecordsUpdated)
			return ApplyManifestEdit(out, p)
		},
	}
}
