package editor

import (
	"fmt"
	"sort"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/log"
)

// SetProfileCode sets a profile's code name ("" clears it).
func SetProfileCode(e *Editor, profile, code string) error {
	m, err := e.load()
	if err != nil {
		return err
	}
	p, ok := m.Profiles[profile]
	if !ok {
		return fmt.Errorf("unknown profile: %q", profile)
	}
	p.Code = code
	m.SetProfile(profile, p)
	if err := m.Validate(); err != nil {
		return err
	}
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "profile.code", log.Field{Key: "profile", Value: profile}, log.Field{Key: "code", Value: code})
	return nil
}

// SetNaming changes defaults.naming through edit, and saves it once the
// manifest still validates.
func SetNaming(e *Editor, edit func(*manifest.Naming)) error {
	m, err := e.load()
	if err != nil {
		return err
	}
	n := manifest.Naming{}
	if m.Defaults.Naming != nil {
		n = *m.Defaults.Naming
	}
	edit(&n)
	if n.Alias == "" && n.KeyName == "" && len(n.Envs) == 0 && len(n.Roles) == 0 && n.Enforce == "" {
		m.Defaults.Naming = nil
	} else {
		m.Defaults.Naming = &n
	}
	if err := m.Validate(); err != nil {
		return err
	}
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "naming.set")
	return nil
}

// SetKeyTemplate makes tpl the key-name template. Every host whose key name
// the new template would change keeps the name it has now, written into the
// manifest as its key_name, so no key already minted is renamed or left
// behind: only hosts added from now on are named by tpl. It returns the hosts
// it pinned, as profile/alias.
func SetKeyTemplate(e *Editor, tpl string) ([]string, error) {
	m, err := e.load()
	if err != nil {
		return nil, err
	}
	before := map[string]string{}
	for _, name := range m.ProfileNames() {
		for _, h := range m.Profiles[name].Hosts {
			if kn, err := m.ResolvedKeyName(name, h); err == nil {
				before[name+"/"+h.Alias] = kn
			}
		}
	}
	n := manifest.Naming{}
	if m.Defaults.Naming != nil {
		n = *m.Defaults.Naming
	}
	n.KeyName = tpl
	m.Defaults.Naming = &n
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var pinned []string
	for _, name := range m.ProfileNames() {
		p := m.Profiles[name]
		changed := false
		for i, h := range p.Hosts {
			old, ok := before[name+"/"+h.Alias]
			if !ok || p.KeyScope == "shared" || (h.KeyName != nil && *h.KeyName != "") {
				continue
			}
			if now, err := m.ResolvedKeyName(name, h); err == nil && now != old {
				kn := old
				p.Hosts[i].KeyName = &kn
				pinned = append(pinned, name+"/"+h.Alias)
				changed = true
			}
		}
		if changed {
			m.SetProfile(name, p)
		}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if err := e.save(m); err != nil {
		return nil, err
	}
	sort.Strings(pinned)
	log.Audit(e.p.AuditLog(), "naming.key_template", log.Field{Key: "template", Value: tpl},
		log.Field{Key: "pinned", Value: len(pinned)})
	return pinned, nil
}

// TakenAliases is every host alias in the manifest, for naming the next one.
func TakenAliases(m *manifest.Manifest) []string {
	var out []string
	for _, name := range m.ProfileNames() {
		for _, h := range m.Profiles[name].Hosts {
			out = append(out, h.Alias)
		}
	}
	return out
}
