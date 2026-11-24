package manifest

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Key roles. An empty Role is a host key (the default): it lives in the
// profile directory. A tool key lives under tools/<tool>/ in that profile.
const (
	RoleHost = "host"
	RoleTool = "tool"
)

// Tool slugs used under profiles/<category>/<profile>/tools/<tool>/.
const (
	ToolGit     = "git"
	ToolDocker  = "docker"
	ToolPloi    = "ploi"
	ToolForge   = "forge"
	ToolAnsible = "ansible"
)

// BuiltinToolSlugs are the well-known tools/<tool>/ folder names.
func BuiltinToolSlugs() []string {
	return []string{ToolGit, ToolDocker, ToolPloi, ToolForge, ToolAnsible}
}

// ToolKind is a more specific binding than the folder slug (git-signing vs a
// future git-ssh tool key, which we do not move into tools/git by default).
const (
	ToolKindGitSigning = "git-signing"
	ToolKindGitSSH     = "git-ssh"
	ToolKindDocker     = "docker"
	ToolKindPloi       = "ploi"
	ToolKindForge      = "forge"
	ToolKindAnsible    = "ansible"
	ToolKindCustom     = "custom"
)

// Built-in service kinds a host or variant may attach. A catalog entry in
// defaults.services may reuse one of these kinds or define a custom name.
const (
	ServiceCloudflareAccess = "cloudflare-access"
	ServiceCloudflareWARP   = "cloudflare-warp"
	ServiceTailscale        = "tailscale"
	ServiceBastion          = "bastion"
	ServiceTeleport         = "teleport"
	ServiceAWSSSM           = "aws-ssm"
	ServiceOnePasswordAgent = "onepassword-agent"
	ServiceSecretive        = "secretive"
	ServiceVPN              = "vpn"
)

// ServiceSpec is one named reachability dependency in defaults.services.
type ServiceSpec struct {
	// Name is how hosts refer to it in services[]; empty equals Kind.
	Name string `json:"name,omitempty"`
	Kind string `json:"kind"`
	// Binary overrides the program looked up on PATH for health checks.
	Binary string `json:"binary,omitempty"`
	// Label is shown in the UI; empty uses a built-in name for Kind.
	Label string `json:"label,omitempty"`
}

// toolSlug is a short folder name: git, docker, ploi-2.
var toolSlug = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// CheckToolSlug reports whether s is a safe tools/<s>/ segment.
func CheckToolSlug(s string) error {
	if !toolSlug.MatchString(s) {
		return fmt.Errorf("tool %q is not a slug (lowercase letter, then letters, digits or '-' up to 32)", s)
	}
	if s == "old" || strings.HasPrefix(s, ".") {
		return fmt.Errorf("tool %q is reserved", s)
	}
	return nil
}

// EffectiveRole is RoleHost when role is empty.
func (k KeySpec) EffectiveRole() string {
	if k.Role == "" {
		return RoleHost
	}
	return k.Role
}

// ToolRel is the path of a tool key relative to its profile directory
// ("tools/git/git-signing"). Empty when the key is not a tool key.
func (k KeySpec) ToolRel() string {
	if k.EffectiveRole() != RoleTool || k.Tool == "" {
		return ""
	}
	return "tools/" + k.Tool + "/" + k.Name
}

func checkKeyPurpose(profileName string, k KeySpec) error {
	where := "key " + k.Name + " in profile " + profileName
	switch k.Role {
	case "", RoleHost, RoleTool:
	default:
		return fmt.Errorf("%s: role %q is not supported (supported: %s, %s, or empty)", where, k.Role, RoleHost, RoleTool)
	}
	if k.EffectiveRole() != RoleTool {
		if k.Tool != "" || k.ToolKind != "" {
			return fmt.Errorf("%s: tool and tool_kind are only for role %q", where, RoleTool)
		}
		return nil
	}
	if k.Tool == "" {
		return fmt.Errorf("%s: a tool key needs a tool slug (git, docker, …)", where)
	}
	if err := CheckToolSlug(k.Tool); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	if k.Path != "" {
		return fmt.Errorf("%s: a tool key cannot also set path (adopted keys stay where they are)", where)
	}
	return nil
}

func checkServices(m *Manifest) error {
	seen := map[string]bool{}
	for i, s := range m.Defaults.Services {
		name := s.Ref()
		if name == "" {
			return fmt.Errorf("defaults.services[%d] needs a name or a kind", i)
		}
		if err := CheckToolSlug(name); err != nil {
			// same slug rules: lowercase path-safe names
			if err := safeSegment("defaults.services name", name); err != nil {
				return fmt.Errorf("defaults.services[%d]: %w", i, err)
			}
		}
		if seen[name] {
			return fmt.Errorf("defaults.services: %q is declared more than once", name)
		}
		seen[name] = true
		if s.Kind == "" {
			return fmt.Errorf("defaults.services %q needs a kind", name)
		}
	}
	for _, name := range m.ProfileNames() {
		p := m.Profiles[name]
		for _, h := range p.Hosts {
			if err := checkServiceRefs(m, name+"/"+h.Alias, h.Services); err != nil {
				return err
			}
			for _, v := range h.Variants {
				if err := checkServiceRefs(m, name+"/"+h.Alias+"#"+v.Name, v.Services); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func checkServiceRefs(m *Manifest, where string, refs []string) error {
	for _, r := range refs {
		if r == "" {
			return fmt.Errorf("%s: an empty services entry", where)
		}
		if err := safeSegment("service", r); err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if !KnownService(m, r) {
			return fmt.Errorf("%s: service %q is not a built-in kind and is not in defaults.services", where, r)
		}
	}
	return nil
}

// Ref is the name hosts use: Name, or Kind when Name is empty.
func (s ServiceSpec) Ref() string {
	if s.Name != "" {
		return s.Name
	}
	return s.Kind
}

// CheckServiceKind reports whether kind is a built-in reachability service.
func CheckServiceKind(kind string) error {
	for _, k := range BuiltinServiceKinds() {
		if kind == k {
			return nil
		}
	}
	return fmt.Errorf("service kind %q is not one of: %s", kind, strings.Join(BuiltinServiceKinds(), ", "))
}

// BuiltinServiceKinds are the kinds authama health-checks and documents.
func BuiltinServiceKinds() []string {
	return []string{
		ServiceCloudflareAccess, ServiceCloudflareWARP, ServiceTailscale,
		ServiceBastion, ServiceTeleport, ServiceAWSSSM,
		ServiceOnePasswordAgent, ServiceSecretive, ServiceVPN,
	}
}

// KnownService reports whether name is a built-in kind or a defaults.services ref.
func KnownService(m *Manifest, name string) bool {
	for _, k := range BuiltinServiceKinds() {
		if name == k {
			return true
		}
	}
	if m == nil {
		return false
	}
	for _, s := range m.Defaults.Services {
		if s.Ref() == name || s.Kind == name {
			return true
		}
	}
	return false
}

// ServiceKind resolves a host/variant services[] entry to a kind.
func ServiceKind(m *Manifest, name string) string {
	if m != nil {
		for _, s := range m.Defaults.Services {
			if s.Ref() == name {
				return s.Kind
			}
		}
	}
	for _, k := range BuiltinServiceKinds() {
		if name == k {
			return k
		}
	}
	return name
}

// ToolDir is the directory of a tool key under the profile: ProfileDir/tools/<tool>.
func ToolDir(m *Manifest, sshDir, profile, tool string) string {
	return filepath.Join(m.ProfileDir(sshDir, profile), "tools", fs.Segment(tool))
}

// KeyRel is the private key's path relative to ~/.ssh, in forward slashes,
// when the key is not adopted.
func KeyRel(m *Manifest, ref KeyRef) string {
	base := m.ProfileRel(ref.Profile)
	if spec, ok := m.KeySpecFor(ref); ok {
		if spec.Path != "" {
			return fs.LocalRel(spec.Path)
		}
		if rel := spec.ToolRel(); rel != "" {
			return base + "/" + rel
		}
	}
	return base + "/" + ref.KeyName
}

// LocateKey describes a key file under the key root: which profile it sits
// in, and the tool slug when it is under tools/<tool>/.
type LocateKey struct {
	Profile string
	Tool    string // empty for a host key at the profile root
	Name    string
}

// LocateKeyPath maps a private-key path under sshDir onto a profile, when
// it sits in that profile's directory or in tools/<tool>/ beneath it.
func LocateKeyPath(m *Manifest, sshDir, abs string) (LocateKey, bool) {
	abs = filepath.Clean(abs)
	for _, name := range m.ProfileNames() {
		dir := filepath.Clean(m.ProfileDir(sshDir, name))
		rel, err := filepath.Rel(dir, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || strings.HasPrefix(rel, "old/") || strings.HasPrefix(rel, ".") {
			continue
		}
		parts := strings.Split(rel, "/")
		switch {
		case len(parts) == 1:
			return LocateKey{Profile: name, Name: parts[0]}, true
		case len(parts) == 3 && parts[0] == "tools" && parts[1] != "old" && !strings.HasPrefix(parts[1], "."):
			return LocateKey{Profile: name, Tool: parts[1], Name: parts[2]}, true
		}
	}
	return LocateKey{}, false
}

// HomeKeyRel is "~/.ssh/" + KeyRel, for plan lines and suggestions.
func HomeKeyRel(p paths.Paths, sshAbs string) string {
	rel, err := filepath.Rel(p.SSHDir, sshAbs)
	if err != nil {
		return sshAbs
	}
	return "~/.ssh/" + filepath.ToSlash(rel)
}
