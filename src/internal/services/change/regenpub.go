package change

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/keysvc"
	"github.com/sangehq/authama/src/internal/services/pubkeys"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// KeyRegenPub is `authama ssh key regen-pub <selection> --yes`: each selected
// key's .pub rebuilt from the key it belongs to.
//
// A preview's copy holds no private key, only an empty stand-in at its path,
// so a .pub derived there would be derived from nothing. Each private key is
// therefore read where it really is, under real, whichever home the change
// writes: deriving the public half reads the private key and copies nothing of
// it, which is what `ssh-keygen -y` does for the CLI. The preview and the
// apply then derive the same public keys, and the confirmation binds to them.
func KeyRegenPub(real paths.Paths, sel keysvc.Selection) Change {
	args := append([]string{}, sel.Selectors...)
	if sel.Category != "" {
		args = append(args, "--category="+sel.Category)
	}
	if sel.All {
		args = append(args, "--all")
	}
	args = append(args, "--yes")
	var subjects []history.Subject
	for _, s := range sel.Selectors {
		if profile, key, ok := strings.Cut(s, "/"); ok {
			subjects = append(subjects, history.Subject{Kind: history.KindProfile, Name: profile},
				history.Subject{Kind: history.KindKey, Name: key})
		} else {
			subjects = append(subjects, history.Subject{Kind: history.KindProfile, Name: s})
		}
	}
	return Change{
		Command: "key regen-pub", Args: args, Subjects: subjects,
		Run: func(p paths.Paths, out io.Writer) error {
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			inv, err := inventory.Load(p.Inventory())
			if err != nil {
				return err
			}
			rows, err := keysvc.New(m, inv, p.SSHDir).Select(sel)
			if err != nil {
				return err
			}
			for i, r := range rows {
				rows[i] = privateFrom(real, p, r)
			}
			results := pubkeys.New(m, inv).Regenerate(rows, false)
			_, _ = fmt.Fprintln(out, pubkeys.Format(results, false))
			return inv.Save(p.Inventory())
		},
	}
}

// privateFrom points a row's private key at the same file under real, and says
// whether it is there. A key in 1Password has no private file to point at.
func privateFrom(real, p paths.Paths, r keysvc.Row) keysvc.Row {
	if r.InOnePassword() || real.SSHDir == p.SSHDir {
		return r
	}
	rel, err := filepath.Rel(p.SSHDir, r.PrivatePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return r
	}
	r.PrivatePath = filepath.Join(real.SSHDir, rel)
	fi, err := os.Stat(r.PrivatePath)
	r.PrivateOnDisk = err == nil && fi.Mode().IsRegular()
	return r
}
