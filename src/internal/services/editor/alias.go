package editor

import (
	"errors"
	"fmt"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/naming"
)

// GenerateAlias names a new host of profile by its naming rules from env and
// role (`ssh host add <profile> --env --role`), and gives the tags the host
// gains: the role and the environment.
func GenerateAlias(m *manifest.Manifest, profile, env, role string) (string, []string, error) {
	if _, ok := m.Profiles[profile]; !ok {
		return "", nil, fmt.Errorf("unknown profile: %q", profile)
	}
	if env == "" && role == "" {
		return "", nil, errors.New("name the host, or give --env and --role to have it named by the naming rules")
	}
	r := manifest.ProfileNamingRules(m, profile)
	code := manifest.ProfileCode(m, profile)
	if code == "" && naming.Required(r.Alias, "code") {
		return "", nil, fmt.Errorf("profile %s has no code to name its servers by; set one (%s ssh profile edit %s --code <code>), "+
			"or let profiles without one use their name (%s ssh naming set --code-fallback profile)",
			profile, brand.Command, profile, brand.Command)
	}
	alias, err := r.Generate(map[string]string{"env": env, "role": role, "code": code}, TakenAliases(m))
	if err != nil {
		return "", nil, err
	}
	var tags []string
	for _, v := range []string{role, env} {
		if v != "" {
			tags = append(tags, v)
		}
	}
	return alias, tags, nil
}
