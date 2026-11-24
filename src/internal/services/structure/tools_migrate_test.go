package structure

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
)

// Tool keys declared under a profile, still in the tools/ folder an older
// layout kept inside it, are offered as moves to the shared
// ~/.ssh/profiles/tools/<tool>/ folder. Applying them moves both halves,
// repoints IdentityFile lines, keeps each key's inventory record (now at the
// new path) and hands the git signing key to sange to retarget ~/.gitconfig.
func TestToolKeysMoveFromTheProfileToTheSharedFolder(t *testing.T) {
	p := home(t, `{"version":1,"defaults":{"key_type":"ed25519","key_scope":"per_service"},"profiles":{
	  "imanimanyara":{"category":"personal","key_scope":"per_service",
	    "keys":[{"name":"git-signing","type":"ed25519","role":"tool","tool":"git","tool_kind":"git-signing"},
	            {"name":"docker-tunnel","role":"tool","tool":"docker","tool_kind":"docker"}],
	    "hosts":[{"alias":"incus","hostname":"incus","user":"imanimanyara","port":22,"key_name":"docker-tunnel"}]}}}`)
	legacy := filepath.Join(p.SSHDir, "profiles", "personal", "imanimanyara", "tools")
	pair(t, filepath.Join(legacy, "git", "git-signing"), pubGit)
	pair(t, filepath.Join(legacy, "docker", "docker-tunnel"), pubDocker)
	write(t, filepath.Join(p.SSHDir, "config"), "Host incus\n  IdentityFile ~/.ssh/profiles/personal/imanimanyara/tools/docker/docker-tunnel\n", 0o600)
	inv := inventory.New()
	inv.Record("SHA256:docker", inventory.KeyRecord{Profile: "imanimanyara", Path: "~/.ssh/profiles/personal/imanimanyara/tools/docker/docker-tunnel", Type: "ed25519",
		Deployments: []inventory.Deployment{{Target: "incus", Method: "authorized_keys", Verified: true}}})
	if err := inv.Save(p.Inventory()); err != nil {
		t.Fatal(err)
	}

	ss, err := Suggest(p)
	if err != nil {
		t.Fatal(err)
	}
	var plan []string
	for _, s := range ss {
		if s.Kind == KindMisplaced {
			plan = append(plan, s.Plan)
		}
	}
	if len(plan) != 2 {
		t.Fatalf("%d misplaced moves, want 2 (git-signing, docker-tunnel): %+v", len(plan), ss)
	}

	var sangeCalls [][]string
	orig := runSange
	t.Cleanup(func() { runSange = orig })
	runSange = func(args ...string) (string, error) {
		sangeCalls = append(sangeCalls, append([]string{}, args...))
		return "updated\n", nil
	}
	moves, err := ParsePlan(strings.Join(plan, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := Apply(p, moves, &buf); err != nil {
		t.Fatal(err, buf.String())
	}
	shared := filepath.Join(p.SSHDir, "profiles", "tools")
	for _, f := range []string{"git/git-signing", "git/git-signing.pub", "docker/docker-tunnel", "docker/docker-tunnel.pub"} {
		if _, err := os.Stat(filepath.Join(shared, filepath.FromSlash(f))); err != nil {
			t.Errorf("not moved to the shared folder: %s", f)
		}
		if _, err := os.Stat(filepath.Join(legacy, filepath.FromSlash(f))); err == nil {
			t.Errorf("still in the profile's tools folder: %s", f)
		}
	}
	cfg, _ := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if !strings.Contains(string(cfg), "IdentityFile ~/.ssh/profiles/tools/docker/docker-tunnel") {
		t.Errorf("IdentityFile not repointed:\n%s", cfg)
	}
	after, err := inventory.Load(p.Inventory())
	if err != nil {
		t.Fatal(err)
	}
	if rec := after.Keys["SHA256:docker"]; rec.Path != "~/.ssh/profiles/tools/docker/docker-tunnel" || len(rec.Deployments) != 1 {
		t.Errorf("the inventory record did not follow the key: %+v", rec)
	}
	if len(sangeCalls) != 1 || !strings.Contains(strings.Join(sangeCalls[0], " "), "profiles/tools/git/git-signing.pub") {
		t.Errorf("the git signing key was not handed to sange: %v", sangeCalls)
	}
}
