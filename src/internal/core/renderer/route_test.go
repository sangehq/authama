package renderer

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/envnames"
)

// The route config is the flat export, owner-only, in the state home; the
// direct route writes none. A host behind a proxy, a VM or a jump has a route.
func TestTheRouteConfigIsTheFlatExport(t *testing.T) {
	mf := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(mf, []byte(`{"version":1,"profiles":{"acme":{"key_scope":"per_service","hosts":[
	  {"alias":"bastion","hostname":"bastion.example","user":"ops"},
	  {"alias":"app","hostname":"10.0.0.5","user":"deploy","port":2222,"raw_options":{"ProxyJump":"bastion"}},
	  {"alias":"edge","hostname":"ssh.example.com","user":"admin","proxy":{"kind":"cloudflared"}}]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(mf)
	if err != nil {
		t.Fatal(err)
	}
	ssh, state := t.TempDir(), filepath.Join(t.TempDir(), "state")
	t.Setenv(envnames.DeployRoute, "")
	path := RouteConfig(m, ssh, state, false, false)
	if path != filepath.Join(state, RouteFileName) {
		t.Fatalf("route config at %q", path)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{"Host bastion", "Host app", "ProxyJump bastion", "Port 2222", "Host edge", "ProxyCommand"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the route config lacks %q:\n%s", want, b)
		}
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	// OpenSSH itself resolves the alias from it (ssh -G parses, never connects).
	if _, err := exec.LookPath("ssh"); err == nil {
		out, err := exec.Command("ssh", "-G", "-F", path, "--", "app").Output()
		if err != nil {
			t.Fatalf("ssh -G: %v", err)
		}
		for _, want := range []string{"hostname 10.0.0.5", "user deploy", "port 2222", "proxyjump bastion"} {
			if !strings.Contains(string(out), want+"\n") {
				t.Errorf("ssh -G app lacks %q", want)
			}
		}
	}
	hosts := m.Profiles["acme"].Hosts
	if HasRoute(hosts[0]) || !HasRoute(hosts[1]) || !HasRoute(hosts[2]) {
		t.Error("HasRoute is wrong")
	}
	if got := RouteConfig(m, ssh, state, false, true); got != "" {
		t.Errorf("direct wrote %q", got)
	}
	t.Setenv(envnames.DeployRoute, "direct")
	if got := RouteConfig(m, ssh, state, false, false); got != "" {
		t.Errorf("the direct route wrote %q", got)
	}
}
