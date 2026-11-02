package change

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keyaudit"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// ResolveReserve turns `key reserve`'s argument - a bare name, ~/.ssh/<name>,
// an absolute path, or any of them with .pub - into the file name directly in
// ~/.ssh, and checks it is a loose keypair authama does not manage. It reads
// only the public half (for the fingerprint).
func ResolveReserve(p paths.Paths, arg string) (name, fingerprint string, err error) {
	arg = strings.TrimSpace(arg)
	if home, herr := os.UserHomeDir(); herr == nil && strings.HasPrefix(arg, "~/") {
		arg = filepath.Join(home, arg[2:])
	}
	arg = strings.TrimSuffix(arg, ".pub")
	name = arg
	if strings.ContainsAny(arg, `/\`) {
		dir, derr := filepath.Abs(filepath.Dir(arg))
		if derr != nil || !sameDir(dir, p.SSHDir) {
			return "", "", fmt.Errorf("%q is not directly in %s: only a key file there can be reserved", arg, p.SSHDir)
		}
		name = filepath.Base(arg)
	}
	if err := manifest.ValidReservedKeyName(name); err != nil {
		return "", "", err
	}
	priv := filepath.Join(p.SSHDir, name)
	if fi, serr := os.Lstat(priv); serr != nil {
		return "", "", fmt.Errorf("~/.ssh/%s: %w", name, serr)
	} else if !fi.Mode().IsRegular() {
		return "", "", fmt.Errorf("~/.ssh/%s is not a regular file", name)
	}
	fingerprint, ferr := keyaudit.PublicFingerprint(p.SSHDir, name)
	if ferr != nil {
		return "", "", fmt.Errorf("~/.ssh/%s.pub is missing or not a public key; recreate it with: ssh-keygen -y -f ~/.ssh/%s > ~/.ssh/%s.pub", name, name, name)
	}
	for _, k := range keyaudit.LooseKeys(p.SSHDir) {
		if k.Name == name && k.CopyOf != "" {
			return "", "", fmt.Errorf("~/.ssh/%s is a copy of ~/.ssh/%s, which authama already manages; nothing to reserve", name, k.CopyOf)
		}
	}
	return name, fingerprint, nil
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// ReservationFor builds the reservation `key reserve` records.
func ReservationFor(forWhat, note, fingerprint string, now time.Time) manifest.ReservedKey {
	return manifest.ReservedKey{For: strings.TrimSpace(forWhat), Note: strings.TrimSpace(note),
		Fingerprint: fingerprint, Since: now.Format("2006-01-02")}
}

// KeyReserve is `authama ssh key reserve <name> [--for X] [--note Y] --yes`:
// keep a loose ~/.ssh key on purpose. Only the manifest changes.
func KeyReserve(name, forWhat, note string) Change {
	args := []string{name}
	if forWhat != "" {
		args = append(args, "--for="+forWhat)
	}
	if note != "" {
		args = append(args, "--note="+note)
	}
	return Change{
		Command: "key reserve", Args: append(args, "--yes"),
		Subjects: []history.Subject{{Kind: history.KindKey, Name: name}},
		Effects: func(p paths.Paths) ([]string, error) {
			if _, _, err := ResolveReserve(p, name); err != nil {
				return nil, err
			}
			return []string{"records ~/.ssh/" + name + " as reserved in manifest.json; the key file is not touched"}, nil
		},
		Run: func(p paths.Paths, out io.Writer) error {
			n, fp, err := ResolveReserve(p, name)
			if err != nil {
				return err
			}
			if err := editor.New(p).ReserveKey(n, ReservationFor(forWhat, note, fp, time.Now())); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "reserved ~/.ssh/%s (%s); doctor reports it as kept on purpose\n", n, fp)
			return nil
		},
	}
}

// KeyUnreserve is `authama ssh key unreserve <name> --yes`: the key is loose
// again (doctor lists it); the file is not touched.
func KeyUnreserve(name string) Change {
	return Change{
		Command: "key unreserve", Args: []string{name, "--yes"},
		Subjects: []history.Subject{{Kind: history.KindKey, Name: name}},
		Run: func(p paths.Paths, out io.Writer) error {
			if _, err := editor.New(p).UnreserveKey(name, false); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "unreserved %s; the key file is untouched\n", name)
			return nil
		},
	}
}
