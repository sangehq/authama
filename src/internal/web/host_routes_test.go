package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/mutation"
)

// confirm previews a change and confirms it, failing on either.
func (c *client) confirm(t *testing.T, f url.Values, want ...string) string {
	t.Helper()
	rec := c.change("/changes/preview", f)
	body := rec.Body.String()
	m := hashField.FindStringSubmatch(body)
	if rec.Code != http.StatusOK || m == nil {
		t.Fatalf("preview of %s: %d\n%.1500s", f.Get("action"), rec.Code, body)
	}
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("the %s preview lacks %s:\n%.1500s", f.Get("action"), w, body)
		}
	}
	f.Set("hash", m[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply of %s: %d\n%.1500s", f.Get("action"), rec.Code, rec.Body.String())
	}
	return body
}

func hostIn(t *testing.T, path, profile, alias string) manifest.Host {
	t.Helper()
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range m.Profiles[profile].Hosts {
		if h.Alias == alias {
			return h
		}
	}
	t.Fatalf("%s has no host %s", profile, alias)
	return manifest.Host{}
}

// The host forms carry what host add, host edit and host variant set take
// from the CLI: extra names, the proxy kinds, a jump host, a timeout, any ssh
// option, the comments, and a host's route variants. The edit form starts
// filled in, so what it posts back unchanged changes nothing, and a name or
// option taken out of it is removed.
func TestTheHostFormsSetRoutesAsTheCLIDoes(t *testing.T) {
	s, p, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	c.confirm(t, url.Values{"action": {"host-add"}, "profile": {"work"}, "alias": {"web0"}, "hostname": {"ssh-web.example.com"},
		"user": {"ploi"}, "port": {"22"}, "aliases": {"web0-cf"}, "proxy": {"cloudflared"},
		"section": {"prod: Cloudflare Access"}, "options": {"ServerAliveInterval=30"}},
		"--alias=web0-cf", "--proxy=cloudflared", "--option=ServerAliveInterval=30", "--section=prod: Cloudflare Access")
	h := hostIn(t, p.Manifest(), "work", "web0")
	if len(h.Aliases) != 1 || h.Proxy == nil || h.Proxy.Kind != "cloudflared" || h.Section == "" || h.RawOptions.Get("ServerAliveInterval") != "30" {
		t.Fatalf("host add did not keep the routes: %+v", h)
	}

	page := c.get("/hosts/web0").Body.String()
	for _, want := range []string{`name="aliases" value="web0-cf"`, `ServerAliveInterval=30</textarea>`, `value="prod: Cloudflare Access"`,
		`value="host-variant-set"`, `Unchanged (cloudflared)`} {
		if !strings.Contains(page, want) {
			t.Errorf("the host page lacks %s", want)
		}
	}

	// The edit form posted back as shown changes nothing but what was typed.
	c.confirm(t, url.Values{"action": {"host-edit"}, "profile": {"work"}, "alias": {"web0"}, "hostname": {"ssh-web.example.com"},
		"user": {"ploi"}, "port": {"22"}, "aliases": {"web0-cf, web0-b"}, "proxy": {""}, "proxy_command": {""},
		"proxy_jump": {""}, "connect_timeout": {"15"}, "options": {""}, "section": {"prod: Cloudflare Access"}, "comment": {"the web box"}},
		"--alias=web0-b", "--connect-timeout=15", "--unset-option=ServerAliveInterval", "--comment=the web box")
	h = hostIn(t, p.Manifest(), "work", "web0")
	if strings.Join(h.Aliases, ",") != "web0-cf,web0-b" || h.RawOptions.Get("ServerAliveInterval") != "" ||
		h.RawOptions.Get("ConnectTimeout") != "15" || h.Comment != "the web box" || h.Proxy == nil {
		t.Fatalf("host edit: %+v", h)
	}

	// The WARP twin: its own address, ProxyCommand none and a timeout.
	c.confirm(t, url.Values{"action": {"host-variant-set"}, "profile": {"work"}, "alias": {"web0"}, "name": {"warp"},
		"hostname": {"10.116.0.3"}, "port": {""}, "proxy": {"none"}, "proxy_command": {""}, "proxy_jump": {""},
		"connect_timeout": {"10"}, "options": {""}, "section": {"prod: private IPs over WARP"}, "comment": {""}},
		"variant set", "--hostname=10.116.0.3", "--proxy=none", "--connect-timeout=10")
	b, err := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(b)
	if cfg, err := filepath.Glob(filepath.Join(p.SSHDir, "profiles", "*", "config")); err == nil {
		for _, f := range cfg {
			if x, err := os.ReadFile(f); err == nil {
				rendered += string(x)
			}
		}
	}
	for _, want := range []string{"Host web0 web0-cf web0-b\n", "    # the web box\n", "# prod: private IPs over WARP\nHost warp-web0\n",
		"    HostName 10.116.0.3\n", "    ProxyCommand none\n", "    ConnectTimeout 10\n"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the rendered config lacks %q:\n%s", want, rendered)
		}
	}
	if page := c.get("/hosts/web0").Body.String(); !strings.Contains(page, "warp-web0") || !strings.Contains(page, `value="host-variant-remove"`) {
		t.Error("the host page does not list its variant")
	}
	c.confirm(t, url.Values{"action": {"host-variant-remove"}, "profile": {"work"}, "alias": {"web0"}, "name": {"warp"}})
	if h = hostIn(t, p.Manifest(), "work", "web0"); len(h.Variants) != 0 {
		t.Errorf("the variant stayed: %+v", h.Variants)
	}

	// What the CLI refuses, the form refuses before any preview.
	for field, bad := range map[string]string{"options": "ProxyCommand=sh -c id", "proxy": "socks", "aliases": "../x",
		"connect_timeout": "-1", "proxy_command": "nc %h %p; rm -rf ~"} {
		f := url.Values{"action": {"host-edit"}, "profile": {"work"}, "alias": {"web0"}, field: {bad}}
		if rec := c.change("/changes/preview", f); strings.Contains(rec.Body.String(), `name="hash"`) {
			t.Errorf("%s %q was offered for confirmation", field, bad)
		}
	}
	if f := (url.Values{"action": {"host-edit"}, "profile": {"work"}, "alias": {"web0"}, "options": {"Bogus=1"}}); strings.Contains(
		c.change("/changes/preview", f).Body.String(), `name="hash"`) {
		t.Error("an option OpenSSH does not know was offered")
	}
}
