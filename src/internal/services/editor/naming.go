package editor

import (
	"fmt"
	"sort"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/log"
)

// keepKeyNames runs change on m, then gives every host whose derived key name
// the change would alter the name it had before, written into the manifest as
// its key_name. A code, a rule or a key-name template is changed this way, so
// no key already minted is renamed or left behind; only hosts added afterwards
// take the new name. It returns the hosts it kept, as profile/alias.
func keepKeyNames(m *manifest.Manifest, change func() error) ([]string, error) {
	before := map[string]string{}
	for _, name := range m.ProfileNames() {
		for _, h := range m.Profiles[name].Hosts {
			if kn, err := m.ResolvedKeyName(name, h); err == nil {
				before[name+"/"+h.Alias] = kn
			}
		}
	}
	if err := change(); err != nil {
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var kept []string
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
				kept = append(kept, name+"/"+h.Alias)
				changed = true
			}
		}
		if changed {
			m.SetProfile(name, p)
		}
	}
	sort.Strings(kept)
	return kept, m.Validate()
}

// SetProfileCode sets a profile's code name ("" clears it), keeping every
// existing host's key name.
func SetProfileCode(e *Editor, profile, code string) ([]string, error) {
	m, err := e.load()
	if err != nil {
		return nil, err
	}
	if _, ok := m.Profiles[profile]; !ok {
		return nil, fmt.Errorf("unknown profile: %q", profile)
	}
	kept, err := keepKeyNames(m, func() error {
		p := m.Profiles[profile]
		p.Code = code
		m.SetProfile(profile, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := e.save(m); err != nil {
		return nil, err
	}
	log.Audit(e.p.AuditLog(), "profile.code", log.Field{Key: "profile", Value: profile}, log.Field{Key: "code", Value: code})
	return kept, nil
}

func emptyNaming(n manifest.Naming) bool {
	return n.Alias == "" && n.KeyName == "" && n.Envs == nil && n.Roles == nil && n.Enforce == "" &&
		n.Start == 0 && n.CodeFallback == ""
}

// SetNaming changes the naming rules through edit: the manifest's
// (defaults.naming) when profile is "", else that profile's own. Every
// existing host keeps its key name.
func SetNaming(e *Editor, profile string, edit func(*manifest.Naming)) ([]string, error) {
	m, err := e.load()
	if err != nil {
		return nil, err
	}
	if profile != "" {
		if _, ok := m.Profiles[profile]; !ok {
			return nil, fmt.Errorf("unknown profile: %q", profile)
		}
	}
	kept, err := keepKeyNames(m, func() error {
		var cur *manifest.Naming
		if profile == "" {
			cur = m.Defaults.Naming
		} else {
			cur = m.Profiles[profile].Naming
		}
		n := manifest.Naming{}
		if cur != nil {
			n = *cur
		}
		edit(&n)
		var next *manifest.Naming
		if !emptyNaming(n) {
			next = &n
		}
		if profile == "" {
			m.Defaults.Naming = next
		} else {
			p := m.Profiles[profile]
			p.Naming = next
			m.SetProfile(profile, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := e.save(m); err != nil {
		return nil, err
	}
	log.Audit(e.p.AuditLog(), "naming.set", log.Field{Key: "profile", Value: profile}, log.Field{Key: "kept", Value: len(kept)})
	return kept, nil
}

// SetKeyTemplate makes tpl the key-name template, the manifest's when profile
// is "", else that profile's own, keeping every existing host's key name: only
// hosts added from now on are named by tpl. It returns the hosts it kept.
func SetKeyTemplate(e *Editor, profile, tpl string) ([]string, error) {
	return SetNaming(e, profile, func(n *manifest.Naming) { n.KeyName = tpl })
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
