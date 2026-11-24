package manifest

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// toolsManifest has a personal profile owning two tool keys and an org
// profile with a host key: the layout ~/.ssh/profiles/tools/<tool>/ holds
// the tool keys, while each stays declared under (linked to) its profile.
const toolsManifest = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
  "me":{"category":"personal","key_scope":"per_service","keys":[
    {"name":"git-signing","role":"tool","tool":"git","tool_kind":"git-signing"},
    {"name":"docker-tunnel","role":"tool","tool":"docker","tool_kind":"docker"}],
   "hosts":[{"alias":"incus","hostname":"incus","user":"me","key_name":"docker-tunnel"}]},
  "acme":{"category":"orgs","key_scope":"per_service","hosts":[{"alias":"web","hostname":"203.0.113.9","user":"deploy"}]}}}`

func loadTools(t *testing.T, raw string) (*Manifest, error) {
	t.Helper()
	var m Manifest
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	return &m, m.Validate()
}

func TestToolKeysLiveInTheSharedToolsFolder(t *testing.T) {
	m, err := loadTools(t, toolsManifest)
	if err != nil {
		t.Fatal(err)
	}
	ref := KeyRef{Profile: "me", KeyName: "git-signing"}
	if got := KeyRel(m, ref); got != "profiles/tools/git/git-signing" {
		t.Errorf("KeyRel = %q, want profiles/tools/git/git-signing", got)
	}
	if got := m.KeyPath("/h/.ssh", ref); got != filepath.FromSlash("/h/.ssh/profiles/tools/git/git-signing") {
		t.Errorf("KeyPath = %q", got)
	}
	if got := m.IdentityFile("me", "docker-tunnel"); got != "~/.ssh/profiles/tools/docker/docker-tunnel" {
		t.Errorf("IdentityFile = %q", got)
	}
	if got := ToolDir(m, "/h/.ssh", "me", "git"); got != filepath.FromSlash("/h/.ssh/profiles/tools/git") {
		t.Errorf("ToolDir = %q", got)
	}
	// A host key is unchanged: in its profile's folder.
	if got := m.IdentityFile("acme", "acme_web-ed25519"); !strings.HasPrefix(got, "~/.ssh/profiles/orgs/acme/") {
		t.Errorf("a host key moved: %q", got)
	}
}

// A file in the shared folder maps back to the profile that declares it;
// one in the old per-profile tools/ folder is still recognised, so Structure
// can offer to move it.
func TestLocateKeyPathFindsToolKeysOldAndNew(t *testing.T) {
	m, err := loadTools(t, toolsManifest)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]LocateKey{
		"/h/.ssh/profiles/tools/git/git-signing":                  {Profile: "me", Tool: "git", Name: "git-signing"},
		"/h/.ssh/profiles/personal/me/tools/docker/docker-tunnel": {Profile: "me", Tool: "docker", Name: "docker-tunnel"},
	} {
		got, ok := LocateKeyPath(m, "/h/.ssh", filepath.FromSlash(path))
		if !ok || got != want {
			t.Errorf("%s: %+v %v, want %+v", path, got, ok, want)
		}
	}
	if _, ok := LocateKeyPath(m, "/h/.ssh", filepath.FromSlash("/h/.ssh/profiles/tools/git/nobody-declares")); ok {
		t.Error("a key no profile declares was given an owner")
	}
}

func TestTheToolsFolderNameIsReserved(t *testing.T) {
	for name, raw := range map[string]string{
		"a category named tools":               `{"version":1,"profiles":{"x":{"category":"tools","key_scope":"per_service","hosts":[]}}}`,
		"an uncategorised profile named tools": `{"version":1,"profiles":{"tools":{"key_scope":"per_service","hosts":[]}}}`,
		"two owners of one tool key": `{"version":1,"profiles":{
		  "a":{"category":"personal","key_scope":"per_service","keys":[{"name":"git-signing","role":"tool","tool":"git"}],"hosts":[]},
		  "b":{"category":"orgs","key_scope":"per_service","keys":[{"name":"git-signing","role":"tool","tool":"git"}],"hosts":[]}}}`,
	} {
		if _, err := loadTools(t, raw); err == nil || !strings.Contains(err.Error(), "tools") {
			t.Errorf("%s: accepted (%v)", name, err)
		}
	}
	// The same key name for two different tools is two files, and fine.
	ok := `{"version":1,"profiles":{
	  "a":{"category":"personal","key_scope":"per_service","keys":[{"name":"deploy","role":"tool","tool":"git"}],"hosts":[]},
	  "b":{"category":"orgs","key_scope":"per_service","keys":[{"name":"deploy","role":"tool","tool":"docker"}],"hosts":[]}}}`
	if _, err := loadTools(t, ok); err != nil {
		t.Errorf("one name under two tools was refused: %v", err)
	}
}
