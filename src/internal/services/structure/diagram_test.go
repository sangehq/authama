package structure

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagramDrawsTheFolderWithItsOwners(t *testing.T) {
	p := home(t, `{"version":1,"defaults":{"key_type":"ed25519","key_scope":"per_service"},"profiles":{
	  "me":{"category":"personal","key_scope":"per_service",
	    "keys":[{"name":"git-signing","role":"tool","tool":"git","tool_kind":"git-signing"}],
	    "hosts":[{"alias":"github-me","hostname":"github.com","user":"git","key_name":"me_github-ed25519"}]},
	  "acme":{"category":"orgs","key_scope":"per_service","hosts":[{"alias":"web","hostname":"203.0.113.9","user":"deploy","key_name":"acme_web-ed25519"}]}}}`)
	ssh := p.SSHDir
	write(t, filepath.Join(ssh, "config"), "Include profiles/personal/config\n", 0o600)
	write(t, filepath.Join(ssh, "known_hosts"), "", 0o600)
	write(t, filepath.Join(ssh, "profiles", "personal", "config"), "", 0o600)
	write(t, filepath.Join(ssh, "profiles", "orgs", "config"), "", 0o600)
	pair(t, filepath.Join(ssh, "profiles", "personal", "me", "me_github-ed25519"), pubGit)
	pair(t, filepath.Join(ssh, "profiles", "orgs", "acme", "acme_web-ed25519"), pubDocker)
	pair(t, filepath.Join(ssh, "profiles", "tools", "git", "git-signing"), pubGit)
	write(t, filepath.Join(ssh, "profiles", "personal", "me", "old", "rotated"), "x", 0o600)
	write(t, filepath.Join(ssh, ".DS_Store"), "x", 0o600)

	d, err := Diagram(p)
	if err != nil {
		t.Fatal(err)
	}
	out := d.String()
	for _, want := range []string{
		"~/.ssh/\n",
		"├── config", "# root config, includes the profile configs",
		"├── known_hosts", "# trusted host fingerprints",
		"me_github-ed25519{,.pub}", "# used by github-me",
		"acme_web-ed25519{,.pub}", "# used by web",
		"# host entries for orgs profiles",
		"└── tools/", "# no config, used directly by tools",
		"git-signing{,.pub}", "# git-signing, owned by personal/me",
		"2 profile groups · 2 owners · 3 key pairs (6 key files) · 4 config/host files",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the diagram lacks %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"old/", ".DS_Store", "rotated"} {
		if strings.Contains(out, not) {
			t.Errorf("the diagram shows %q, which it skips", not)
		}
	}
	// tools/ is drawn last under profiles/, after the categories.
	if strings.Index(out, "tools/") < strings.Index(out, "personal/") {
		t.Error("tools/ is drawn before the categories")
	}
}
