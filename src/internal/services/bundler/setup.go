package bundler

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/agepass"
)

// PassphraseCipher encrypts with a passphrase in age's own format (see
// util/agepass), for a full setup export a person carries to another machine:
// there is no recipient key there yet to encrypt to. The recipient and
// identity arguments are ignored.
type PassphraseCipher struct{ Passphrase []byte }

func (c PassphraseCipher) Encrypt(dst io.Writer, src io.Reader, _ string) error {
	return agepass.Encrypt(dst, src, c.Passphrase, 0)
}

func (c PassphraseCipher) Decrypt(dst io.Writer, src io.Reader, _ string) error {
	return agepass.Decrypt(dst, src, c.Passphrase)
}

// SecretsMember is the member of a full setup export that holds token values,
// as .env lines. A backup bundle never has it; LayDown never writes it: the
// importer takes it out of the opened archive and merges it into .env itself.
const SecretsMember = "secrets/tokens.env"

// setupMembers are the members a full setup export carries beyond a backup
// bundle's own: the settings, the token config (names only) and the token values.
var setupMembers = []string{configPrefix + "tokens.json", configPrefix + "authama.json", SecretsMember}

// IsSetupMember reports whether name is one of the members only a full setup
// export carries.
func IsSetupMember(name string) bool {
	for _, s := range setupMembers {
		if name == s {
			return true
		}
	}
	return false
}

// WithSetupMembers adds members to what Bundle and WriteFile archive: each is
// named in setupMembers and given as its content, so the token values are
// never written to a file to be read back.
func (b *Bundler) WithSetupMembers(extra map[string][]byte) (*Bundler, error) {
	for name := range extra {
		if !IsSetupMember(name) {
			return nil, fmt.Errorf("not a setup member: %s", name)
		}
	}
	b.extra = extra
	return b, nil
}

// addBytes writes one in-memory member, owner-only.
func addBytes(tw *tar.Writer, arc string, data []byte, now time.Time) error {
	hdr := &tar.Header{Name: arc, Mode: 0o600, Size: int64(len(data)), ModTime: now, Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

// writeExtra appends the setup members, in name order.
func (b *Bundler) writeExtra(tw *tar.Writer) ([]string, error) {
	names := make([]string, 0, len(b.extra))
	for n := range b.extra {
		names = append(names, n)
	}
	sort.Strings(names)
	now := time.Now()
	for _, n := range names {
		if err := addBytes(tw, n, b.extra[n], now); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// WriteFile writes the archive, encrypted, to exactly path, which must not
// exist yet; nothing beside it. A half-written file is removed.
func (b *Bundler) WriteFile(path string) (BundleResult, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return BundleResult{}, err
	}
	contents, err := b.encryptTo(path, "")
	if errors.Is(err, os.ErrExist) {
		return BundleResult{}, fmt.Errorf("%s already exists; choose another name, or move it away first", path)
	}
	if err != nil {
		_ = os.Remove(path)
		return BundleResult{}, err
	}
	sha, err := sha256File(path)
	if err != nil {
		return BundleResult{}, err
	}
	return BundleResult{AgePath: path, SHA256: sha, Contents: contents}, nil
}

// Take removes the member name from the opened archive and returns its content.
func (o *Opened) Take(name string) ([]byte, bool) {
	for i, m := range o.members {
		if m.name == name {
			o.members = append(o.members[:i], o.members[i+1:]...)
			return m.data, true
		}
	}
	return nil, false
}

// MemberState is one key file in an opened archive against what is on disk.
type MemberState struct {
	Name  string // ssh/profiles/<rel>, as archived
	Dest  string // where it lands, relative to ~/.ssh
	State string // "new", "same" or "differs"
}

// KeyStates compares every key file in o with the file it would replace.
// Every member is vetted first, as LayDown does.
func (b *Bundler) KeyStates(o *Opened) ([]MemberState, error) {
	var out []MemberState
	for _, m := range o.members {
		if err := checkMember(m.name); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(b.sshDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			root = nil
		} else {
			return nil, err
		}
	}
	if root != nil {
		defer func() { _ = root.Close() }()
	}
	for _, m := range o.members {
		rel, ok := strings.CutPrefix(m.name, sshPrefix)
		if !ok {
			continue
		}
		dest := b.keyRoot + strings.TrimPrefix(rel, manifest.DefaultKeyRoot)
		st := MemberState{Name: m.name, Dest: dest, State: "new"}
		if root != nil {
			if cur, err := readIn(root, dest); err == nil {
				st.State = "differs"
				if string(cur) == string(m.data) {
					st.State = "same"
				}
			}
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Drop removes the named members from the opened archive.
func (o *Opened) Drop(names map[string]bool) {
	kept := o.members[:0]
	for _, m := range o.members {
		if !names[m.name] {
			kept = append(kept, m)
		}
	}
	o.members = kept
}
