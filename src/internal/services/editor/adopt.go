package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/log"
)

// AdoptKey declares a key that already exists where the user keeps it - say
// ~/.ssh/id_ed25519 - as profile's key name, without moving or renaming it.
// rel is the private key's path relative to ~/.ssh. With hostAlias it is also
// wired to that host. The file must exist, be a private key, and resolve
// inside ~/.ssh (a symlink out of it is refused).
func AdoptKey(e *Editor, profile, name, rel, hostAlias string) error {
	if err := manifest.CheckAdoptedPath(rel); err != nil {
		return err
	}
	abs := filepath.Join(e.p.SSHDir, filepath.FromSlash(rel))
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("no key at ~/.ssh/%s: %w", rel, err)
	}
	sshReal, err := filepath.EvalSymlinks(e.p.SSHDir)
	if err != nil {
		return err
	}
	if inside, _ := filepath.Rel(sshReal, real); inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return fmt.Errorf("~/.ssh/%s resolves outside ~/.ssh (%s)", rel, real)
	}
	head := make([]byte, 64)
	f, err := os.Open(real)
	if err != nil {
		return err
	}
	n, _ := f.Read(head)
	_ = f.Close()
	if !strings.Contains(string(head[:n]), "PRIVATE KEY") {
		return fmt.Errorf("~/.ssh/%s is not a private key", rel)
	}
	m, err := e.load()
	if err != nil {
		return err
	}
	p, ok := m.Profiles[profile]
	if !ok {
		return fmt.Errorf("unknown profile: %q", profile)
	}
	for _, k := range p.Keys {
		if k.Name == name {
			return fmt.Errorf("profile %q already declares key %q", profile, name)
		}
	}
	if hostAlias != "" {
		if p.KeyScope == "shared" {
			return fmt.Errorf("profile %q is shared - every host uses its key_name", profile)
		}
		idx, host, err := findHost(m, profile, hostAlias)
		if err != nil {
			return err
		}
		host.KeyName = &name
		p.Hosts[idx] = host
	}
	p.Keys = append(p.Keys, manifest.KeySpec{Name: name, Path: rel})
	m.SetProfile(profile, p)
	// Validates the whole manifest, so a second key adopting the same file,
	// or a 1Password profile, is refused before anything is written.
	if err := m.Validate(); err != nil {
		return err
	}
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "key.adopt",
		log.Field{Key: "profile", Value: profile}, log.Field{Key: "key", Value: name},
		log.Field{Key: "path", Value: "~/.ssh/" + rel}, log.Field{Key: "host", Value: hostAlias})
	return nil
}
