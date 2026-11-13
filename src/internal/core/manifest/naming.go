package manifest

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/sangehq/authama/src/internal/core/key"
	"github.com/sangehq/authama/src/internal/core/naming"
)

// Naming is defaults.naming: how servers and their keys are named. Every
// field is optional; NamingRules fills the defaults.
type Naming struct {
	// Alias is the alias template: {code}-{env}-{role}-{nn} by default.
	Alias string `json:"alias,omitempty"`
	// KeyName is the key-name template: {profile}_{alias}-{algo} by default,
	// the name every release before templates derived.
	KeyName string `json:"key_name,omitempty"`
	// Envs and Roles, when set, are the values {env} and {role} may take.
	Envs  []string `json:"envs,omitempty"`
	Roles []string `json:"roles,omitempty"`
	// Enforce is what adding a host whose alias breaks the rules does: off
	// (the default), warn, or error. Hosts already there are only reported.
	Enforce string `json:"enforce,omitempty"`
	// Start is the first number a generated alias takes: 0 (00, the
	// default) or 1 (01).
	Start int `json:"start,omitempty"`
	// CodeFallback is the {code} of a profile with no code of its own: none
	// (the default) or profile, its name.
	CodeFallback string `json:"code_fallback,omitempty"`
}

func (n *Naming) rules() naming.Rules {
	if n == nil {
		return naming.Rules{}
	}
	return naming.Rules{Alias: n.Alias, KeyName: n.KeyName, Envs: n.Envs, Roles: n.Roles, Enforce: n.Enforce,
		Start: n.Start, CodeFallback: n.CodeFallback}
}

// NamingRules are m's naming rules with the defaults filled in. A nil m has
// the defaults.
func NamingRules(m *Manifest) naming.Rules {
	if m == nil {
		return naming.Rules{}.WithDefaults()
	}
	return m.Defaults.Naming.rules().WithDefaults()
}

// ProfileNamingRules are the rules a profile's servers are named by: the
// manifest's, with the profile's own naming over them.
func ProfileNamingRules(m *Manifest, profile string) naming.Rules {
	r := NamingRules(m)
	if m != nil {
		if p, ok := m.Profiles[profile]; ok && p.Naming != nil {
			r = r.Over(p.Naming.rules()).WithDefaults()
		}
	}
	return r
}

// ProfileCode is a profile's {code}: its own code, or what the rules' code
// fallback gives it ("" for none).
func ProfileCode(m *Manifest, profile string) string {
	return ProfileNamingRules(m, profile).Code(profile, m.Profiles[profile].Code)
}

var profileCode = regexp.MustCompile(`^[a-z0-9]+$`)

func checkNaming(m *Manifest) error {
	if err := NamingRules(m).Check(); err != nil {
		return err
	}
	owner := map[string]string{}
	for _, name := range m.ProfileNames() {
		if err := ProfileNamingRules(m, name).Check(); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
		c := m.Profiles[name].Code
		if c == "" {
			continue
		}
		if !profileCode.MatchString(c) {
			return fmt.Errorf("profile %q code %q: use lowercase letters and digits, such as simbi", name, c)
		}
		if other, ok := owner[c]; ok {
			return fmt.Errorf("profiles %q and %q both have the code %q; a code names one organisation", other, name, c)
		}
		owner[c] = name
	}
	return nil
}

// derivedKeyName is a host's key name when the manifest does not name one:
// the key-name template rendered for the host, or, with the default template
// (or an alias the template cannot read its parts from), the name every
// release before templates derived.
func derivedKeyName(m *Manifest, profileName, alias string) (string, error) {
	algo := m.Defaults.KeyType
	if algo == "" {
		algo = "ed25519"
	}
	r := ProfileNamingRules(m, profileName)
	if r.KeyName == naming.DefaultKeyName {
		return key.DeriveKeyName(profileName, alias, algo)
	}
	prof := strings.ReplaceAll(key.NormalizeSegment(profileName), "-", "")
	name, err := r.RenderKeyName(prof, key.NormalizeSegment(alias), ProfileCode(m, profileName), algo)
	if errors.Is(err, naming.ErrNoParts) {
		return key.DeriveKeyName(profileName, alias, algo)
	}
	if err != nil {
		return "", err
	}
	if err := safeSegment("key name", name); err != nil {
		return "", err
	}
	return name, nil
}

// NamingViolation says why a host's alias does not follow m's rules, or "".
func NamingViolation(m *Manifest, profileName, alias string) string {
	return ProfileNamingRules(m, profileName).Violation(alias, ProfileCode(m, profileName))
}
