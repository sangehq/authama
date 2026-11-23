package editor

import (
	"fmt"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/log"
)

// SetProfileGit sets (or, with nil, clears) the git identity a profile
// commits as and how it signs. It changes the manifest only; `ssh git sign
// apply` renders it.
func (e *Editor) SetProfileGit(name string, g *manifest.ProfileGit) error {
	m, err := e.load()
	if err != nil {
		return err
	}
	p, ok := m.Profiles[name]
	if !ok {
		return fmt.Errorf("unknown profile: %q", name)
	}
	p.Git = g
	m.SetProfile(name, p)
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "profile.git", log.Field{Key: "profile", Value: name})
	return nil
}
