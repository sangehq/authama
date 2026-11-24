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
// profile directory. A tool key lives in the shared tools folder under the key
// root, <key root>/tools/<tool>/, and stays declared under (linked to) the
// profile that owns it.
const (
	RoleHost = "host"
	RoleTool = "tool"
)

// ToolsDir is the folder under the key root that holds every tool key, one
// subfolder per tool: ~/.ssh/profiles/tools/<tool>/<key>. It is reserved, so no
// category or uncategorised profile may take the name.
const ToolsDir = "tools"

// Tool slugs used under profiles/tools/<tool>/.
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

// ToolRel is the path of a tool key relative to the key root
// ("tools/git/git-signing"). Empty when the key is not a tool key.
func (k KeySpec) ToolRel() string {
	if k.EffectiveRole() != RoleTool || k.Tool == "" {
		return ""
	}
	return ToolsDir + "/" + k.Tool + "/" + k.Name
}

// ToolKeyRel is a tool key's path relative to ~/.ssh: the key root's shared
// tools folder, whichever profile owns it ("profiles/tools/git/git-signing").
func ToolKeyRel(m *Manifest, k KeySpec) string {
	if rel := k.ToolRel(); rel != "" {
		return fs.LocalRel(KeyRoot(m)) + "/" + rel
	}
	return ""
}

// checkToolsReserved refuses what would put another folder where the shared
// tools folder is (a category or an uncategorised profile named tools), and
// two profiles declaring the same tool key, which would be one file.
func checkToolsReserved(m *Manifest) error {
	owner := map[string]string{} // tools/<tool>/<name> -> profile
	for _, name := range m.ProfileNames() {
		p := m.Profiles[name]
		if p.Category == ToolsDir {
			return fmt.Errorf("profile %q: category %q is reserved for the shared tool keys folder (%s/%s/<tool>/)", name, ToolsDir, KeyRoot(m), ToolsDir)
		}
		if p.Category == "" && name == ToolsDir {
			return fmt.Errorf("profile %q: an uncategorised profile cannot be named %q, which is reserved for the shared tool keys folder", name, ToolsDir)
		}
		for _, k := range p.Keys {
			rel := k.ToolRel()
			if rel == "" {
				continue
			}
			if other, ok := owner[rel]; ok {
				return fmt.Errorf("profiles %q and %q both declare tool key %s; tool keys share %s/%s/, so give one a distinct name (for example %s-%s)",
					other, name, rel, KeyRoot(m), ToolsDir, name, k.Name)
			}
			owner[rel] = name
		}
	}
	return nil
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

// ToolDir is the directory of a tool's keys: <key root>/tools/<tool>, shared
// by every profile. profile is kept for callers; the folder does not depend
// on it.
func ToolDir(m *Manifest, sshDir, profile, tool string) string {
	_ = profile
	return filepath.Join(sshDir, filepath.FromSlash(fs.LocalRel(KeyRoot(m))), ToolsDir, fs.Segment(tool))
}

// KeyRel is the private key's path relative to ~/.ssh, in forward slashes,
// when the key is not adopted.
func KeyRel(m *Manifest, ref KeyRef) string {
	base := m.ProfileRel(ref.Profile)
	if spec, ok := m.KeySpecFor(ref); ok {
		if spec.Path != "" {
			return fs.LocalRel(spec.Path)
		}
		if rel := ToolKeyRel(m, spec); rel != "" {
			return rel
		}
	}
	return base + "/" + ref.KeyName
}

// LocateKey describes a key file under the key root: which profile it sits
// in (for a tool key, the profile that declares it), and the tool slug when
// it is in a tools/<tool>/ folder.
type LocateKey struct {
	Profile string
	Tool    string // empty for a host key at the profile root
	Name    string
}

// LocateKeyPath maps a private-key path under sshDir onto a profile: a key in
// the shared tools folder to the profile that declares it, else a key in a
// profile's directory, or in the tools/<tool>/ folder an older layout kept
// beneath it (so Structure can offer to move it to the shared folder).
func LocateKeyPath(m *Manifest, sshDir, abs string) (LocateKey, bool) {
	abs = filepath.Clean(abs)
	toolsRoot := filepath.Join(sshDir, filepath.FromSlash(fs.LocalRel(KeyRoot(m))), ToolsDir)
	if rel, err := filepath.Rel(toolsRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 2 || parts[0] == "old" || strings.HasPrefix(parts[0], ".") {
			return LocateKey{}, false
		}
		for _, name := range m.ProfileNames() {
			for _, k := range m.Profiles[name].Keys {
				if k.EffectiveRole() == RoleTool && k.Tool == parts[0] && k.Name == parts[1] {
					return LocateKey{Profile: name, Tool: parts[0], Name: parts[1]}, true
				}
			}
		}
		return LocateKey{}, false
	}
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
