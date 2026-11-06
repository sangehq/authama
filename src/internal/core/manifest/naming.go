package manifest

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/simtabi/authama/src/internal/core/key"
	"github.com/simtabi/authama/src/internal/core/naming"
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
}

// NamingRules are m's naming rules with the defaults filled in. A nil m has
// the defaults.
func NamingRules(m *Manifest) naming.Rules {
	var r naming.Rules
	if m != nil && m.Defaults.Naming != nil {
		n := m.Defaults.Naming
		r = naming.Rules{Alias: n.Alias, KeyName: n.KeyName, Envs: n.Envs, Roles: n.Roles, Enforce: n.Enforce}
	}
	return r.WithDefaults()
}

var profileCode = regexp.MustCompile(`^[a-z0-9]+$`)

func checkNaming(m *Manifest) error {
	if err := NamingRules(m).Check(); err != nil {
		return err
	}
	owner := map[string]string{}
	for _, name := range m.ProfileNames() {
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
	r := NamingRules(m)
	if r.KeyName == naming.DefaultKeyName {
		return key.DeriveKeyName(profileName, alias, algo)
	}
	prof := strings.ReplaceAll(key.NormalizeSegment(profileName), "-", "")
	name, err := r.RenderKeyName(prof, key.NormalizeSegment(alias), m.Profiles[profileName].Code, algo)
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
	return NamingRules(m).Violation(alias, m.Profiles[profileName].Code)
}
