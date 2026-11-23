package editor

import (
	"fmt"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/log"
)

// UpsertService adds or replaces a catalog entry in defaults.services.
func (e *Editor) UpsertService(spec manifest.ServiceSpec) error {
	name := spec.Ref()
	if name == "" {
		return fmt.Errorf("a service needs a name or a kind")
	}
	if err := manifest.CheckServiceKind(spec.Kind); err != nil {
		return err
	}
	m, err := e.load()
	if err != nil {
		return err
	}
	found := false
	for i, s := range m.Defaults.Services {
		if s.Ref() == name {
			m.Defaults.Services[i] = spec
			found = true
			break
		}
	}
	if !found {
		m.Defaults.Services = append(m.Defaults.Services, spec)
	}
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "service.upsert", log.Field{Key: "name", Value: name}, log.Field{Key: "kind", Value: spec.Kind})
	return nil
}

// RemoveService drops a catalog entry. Hosts that still name it are left
// as-is: Validate reports the dangling ref until they are edited.
func (e *Editor) RemoveService(name string) error {
	name = strings.TrimSpace(name)
	m, err := e.load()
	if err != nil {
		return err
	}
	var out []manifest.ServiceSpec
	found := false
	for _, s := range m.Defaults.Services {
		if s.Ref() == name {
			found = true
			continue
		}
		out = append(out, s)
	}
	if !found {
		return fmt.Errorf("no catalog service %q", name)
	}
	m.Defaults.Services = out
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "service.remove", log.Field{Key: "name", Value: name})
	return nil
}
