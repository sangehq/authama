package renderer

import (
	"os"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

func pinCloudflared(t *testing.T, path string) {
	t.Helper()
	prev := CloudflaredPath
	CloudflaredPath = func() string { return path }
	t.Cleanup(func() { CloudflaredPath = prev })
}

func loadRoutesManifest(t *testing.T) *manifest.Manifest {
	t.Helper()
	raw, err := os.ReadFile("testdata/routes/simtabi.manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	m := mustManifest(t, string(raw))
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	return m
}

// The blocks Sim wrote by hand for simbi - two Cloudflare Access hosts with a
// second name each, and their WARP twins - are what the manifest renders,
// byte for byte, apart from the UserKnownHostsFile line authama writes into
// every host it manages (his oribi blocks show it): that line keeps every
// managed host on the one known_hosts store authama pins into.
func TestTheSimbiRoutesRenderAsWrittenByHand(t *testing.T) {
	pinCloudflared(t, "/opt/homebrew/bin/cloudflared")
	m := loadRoutesManifest(t)
	got, err := RenderCategoryConfig(m, "orgs")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/routes/orgs.config")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("rendered orgs config differs:\n--- got\n%s\n--- want\n%s", got, want)
	}
	block, err := os.ReadFile("testdata/routes/simbi.block")
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.ReplaceAll(got, "    UserKnownHostsFile ~/.ssh/known_hosts\n", "")
	if !strings.HasSuffix(stripped, string(block)+"\n") {
		t.Errorf("the hand-written simbi block is not reproduced byte for byte:\n%s", stripped)
	}
}

// show renders one host with its route variants under their headings.
func TestAHostBlockIncludesItsVariants(t *testing.T) {
	pinCloudflared(t, "/opt/homebrew/bin/cloudflared")
	m := loadRoutesManifest(t)
	h := m.Profiles["simtabi"].Hosts[3]
	out, err := RenderHostBlockFor(m, "simtabi", h)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# simbi prod: Cloudflare Access", "Host simbi-prd-web-00 simbi-web-cf\n",
		"# simbi prod: private IPs over Cloudflare WARP", "Host warp-simbi-prd-web-00\n", "    ProxyCommand none\n",
		"    IdentityFile ~/.ssh/profiles/orgs/simtabi/simtabi_simbi-prd-web-00-ed25519\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// A variant shares the host's key even under per-service keys, where the key
// name is otherwise derived from the alias.
func TestAVariantUsesItsHostsKey(t *testing.T) {
	m := mustManifest(t, `{"version":1,"profiles":{"p":{"hosts":[{"alias":"web","hostname":"web.example","user":"u",
		"key_name":"shared-web","raw_options":{"ProxyJump":"bastion","ServerAliveInterval":"30"},
		"variants":[{"name":"vpn","hostname":"10.0.0.2","proxy":{"kind":"none"}}]}]}}}`)
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	out, err := RenderRootConfig(m, false)
	if err != nil {
		t.Fatal(err)
	}
	vpn := out[strings.Index(out, "Host vpn-web"):]
	for _, want := range []string{"IdentityFile ~/.ssh/profiles/p/shared-web\n", "HostName 10.0.0.2\n",
		"ProxyCommand none\n", "ServerAliveInterval 30\n"} {
		if !strings.Contains(vpn, want) {
			t.Errorf("missing %q in the variant:\n%s", want, vpn)
		}
	}
	if strings.Contains(vpn, "ProxyJump") {
		t.Errorf("the variant's own proxy must replace the host's jump:\n%s", vpn)
	}
}

// A command proxy is an argv: every argument is quoted for the shell ssh runs
// it with, so nothing in one can start a second command.
func TestACommandProxyIsQuotedForTheShell(t *testing.T) {
	h := manifest.Host{Alias: "a", Proxy: &manifest.ProxySpec{Kind: manifest.ProxyCommandKind,
		Command: []string{"/usr/bin/nc", "-X", "5", "-x", "proxy host:1080", "%h", "%p", "x;touch /tmp/p", "it's"}}}
	got := proxyFor(h)
	want := `/usr/bin/nc -X 5 -x 'proxy host:1080' %h %p 'x;touch /tmp/p' 'it'\''s'`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

// A cloudflared path the shell would split is quoted; a plain one is not.
func TestTheCloudflaredPathIsQuotedOnlyWhenItMustBe(t *testing.T) {
	pinCloudflared(t, `C:\Program Files\cloudflared\cloudflared.exe`)
	if got := CloudflaredProxyCommand(); got != `"C:\Program Files\cloudflared\cloudflared.exe" access ssh --hostname %h` {
		t.Errorf("got %s", got)
	}
	pinCloudflared(t, "/usr/local/bin/cloudflared")
	if got := CloudflaredProxyCommand(); got != "/usr/local/bin/cloudflared access ssh --hostname %h" {
		t.Errorf("got %s", got)
	}
}

// A heading is written once for a run of hosts, and a host with no section of
// its own stays under it.
func TestSectionsHeadRunsOfHosts(t *testing.T) {
	m := mustManifest(t, `{"version":1,"profiles":{"p":{"hosts":[
		{"alias":"a","hostname":"a","user":"u","section":"one","comment":"the first"},
		{"alias":"b","hostname":"b","user":"u"},
		{"alias":"c","hostname":"c","user":"u","section":"one"},
		{"alias":"d","hostname":"d","user":"u","section":"two"}]}}}`)
	out, err := RenderRootConfig(m, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "# one\n") != 1 || strings.Count(out, "# two\n") != 1 {
		t.Errorf("each heading once:\n%s", out)
	}
	if !strings.Contains(out, "# one\nHost a\n    # the first\n    HostName a\n") {
		t.Errorf("heading above, comment inside:\n%s", out)
	}
}
