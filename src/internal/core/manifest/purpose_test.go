package manifest

import (
	"path/filepath"
	"testing"
)

func TestKeyPathToolRole(t *testing.T) {
	t.Parallel()
	m := &Manifest{Version: schemaVersion, Defaults: newDefaults(), Profiles: map[string]Profile{
		"imanimanyara": {
			Category: "personal",
			KeyScope: "per_service",
			Keys: []KeySpec{
				{Name: "git-signing", Role: RoleTool, Tool: ToolGit, ToolKind: ToolKindGitSigning},
				{Name: "docker-tunnel", Role: RoleTool, Tool: ToolDocker, ToolKind: ToolKindDocker},
			},
		},
	}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	ssh := t.TempDir()
	git := m.KeyPath(ssh, KeyRef{Profile: "imanimanyara", KeyName: "git-signing"})
	want := filepath.Join(ssh, "profiles", "personal", "imanimanyara", "tools", "git", "git-signing")
	if git != want {
		t.Fatalf("git path: got %s want %s", git, want)
	}
	if rel := m.KeyRel(KeyRef{Profile: "imanimanyara", KeyName: "git-signing"}); rel != "profiles/personal/imanimanyara/tools/git/git-signing" {
		t.Fatalf("key rel: %s", rel)
	}
	loc, ok := m.LocateKeyPath(ssh, git)
	if !ok || loc.Profile != "imanimanyara" || loc.Tool != "git" || loc.Name != "git-signing" {
		t.Fatalf("locate: %+v ok=%v", loc, ok)
	}
}

func TestServiceRefs(t *testing.T) {
	t.Parallel()
	m := &Manifest{Version: schemaVersion, Defaults: newDefaults(), Profiles: map[string]Profile{
		"simtabi": {
			Category: "orgs",
			KeyScope: "per_service",
			Hosts: []Host{{
				Alias: "simbi-prd-web-00", Hostname: "ssh-web-00.example.com", User: "ploi", Port: 22,
				Services: []string{ServiceCloudflareAccess},
				Variants: []Variant{{Name: "warp", Hostname: "10.116.0.3", Services: []string{ServiceCloudflareWARP}}},
			}},
		},
	}}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	h := m.Profiles["simtabi"]
	h.Hosts = append([]Host(nil), h.Hosts...)
	h.Hosts[0].Services = []string{"not-a-service"}
	m.Profiles["simtabi"] = h
	if err := m.Validate(); err == nil {
		t.Fatal("expected unknown service to fail")
	}
}

func TestToolSlug(t *testing.T) {
	t.Parallel()
	if err := CheckToolSlug("git"); err != nil {
		t.Fatal(err)
	}
	if err := CheckToolSlug("Old"); err == nil {
		t.Fatal("uppercase should fail")
	}
	if err := CheckToolSlug("old"); err == nil {
		t.Fatal("reserved old should fail")
	}
}
