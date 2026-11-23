package encode

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

var moreHosts = []Host{
	{Group: "web", Category: "clients", Env: "prd", Alias: "bastion", Hostname: "198.51.100.1", User: "ops", Port: 22,
		KeyFile: "/home/u/.ssh/profiles/clients/web/web_bastion-ed25519"},
	{Group: "web", Category: "clients", Env: "prd", Alias: "app", Hostname: "10.0.0.5", User: "deploy", Port: 2222,
		KeyFile: "/home/u/.ssh/profiles/clients/web/web_app-ed25519", ProxyJump: "bastion", Tags: []string{"php"}},
	{Group: "ops", Alias: "vault", Hostname: "10.0.0.9", User: "root", Port: 22, Agent: "/tmp/1p.sock", ProxyJump: "jump.example"},
}

func renderMore(t *testing.T, name string) (*ClientExport, string) {
	t.Helper()
	c, ok := Clients[name]
	if !ok {
		t.Fatalf("no client %q", name)
	}
	exp, err := c.Render(moreHosts)
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range exp.Files {
		all.Write(f.Data)
	}
	return exp, all.String()
}

// No new format writes a secret: no password field with a value, no private
// key, only key paths.
func TestMoreClientsWriteNoSecret(t *testing.T) {
	for name := range moreClients {
		exp, body := renderMore(t, name)
		if len(exp.Files) == 0 {
			t.Errorf("%s wrote no file", name)
		}
		for _, bad := range []string{"PRIVATE KEY", "KeyFileContent", "KeyFilePassphrase", "Password=", "<Pass", "password:"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s writes %q", name, bad)
			}
		}
		if len(exp.Notes) == 0 {
			t.Errorf("%s says nothing on finishing the import", name)
		}
		notes := strings.Join(exp.Notes, "\n")
		// The host whose key is in 1Password is named, and the agent is.
		if !strings.Contains(notes, "vault keeps its key in 1Password") && name != "mremoteng" {
			t.Errorf("%s does not say vault uses the agent:\n%s", name, notes)
		}
	}
}

// A ProxyJump goes where the client can express it, and is a note where it
// cannot.
func TestMoreClientsCarryOrExplainProxyJump(t *testing.T) {
	for _, name := range []string{"royalts", "rdm", "mremoteng", "xshell", "winscp", "filezilla", "transmit", "cyberduck"} {
		exp, _ := renderMore(t, name)
		notes := strings.Join(exp.Notes, "\n")
		if !strings.Contains(notes, "app reaches its host through ProxyJump bastion") {
			t.Errorf("%s does not explain app's ProxyJump:\n%s", name, notes)
		}
	}
	exp, body := renderMore(t, "tabby")
	// Tabby's id for the profile named bastion: file-config: and sha256("bastion").
	if !strings.Contains(body, "jumpHost: file-config:"+tabbyID("bastion")) {
		t.Errorf("tabby should point app at the exported bastion's profile:\n%s", body)
	}
	if notes := strings.Join(exp.Notes, "\n"); !strings.Contains(notes, "vault reaches its host through ProxyJump jump.example") ||
		strings.Contains(notes, "app reaches") {
		t.Errorf("tabby should note only the jump host it could not map:\n%s", notes)
	}
	_, body = renderMore(t, "ansible")
	for _, want := range []string{"ansible_ssh_common_args: -o ProxyJump=bastion", "env_prd:", "ungrouped_profiles:", "ansible_port: 2222"} {
		if !strings.Contains(body, want) {
			t.Errorf("ansible inventory lacks %q:\n%s", want, body)
		}
	}
	for _, name := range []string{"blink", "prompt3"} {
		_, body := renderMore(t, name)
		if !strings.Contains(body, "ProxyJump bastion") || !strings.Contains(body, "IdentityFile web_app-ed25519") {
			t.Errorf("%s snippet:\n%s", name, body)
		}
	}
}

// Per-host formats write one file per host, named after it; names that
// would escape the zip are made safe.
func TestPerHostFormatsNameAFilePerHost(t *testing.T) {
	for name, ext := range map[string]string{"xshell": ".xsh", "cyberduck": ".duck"} {
		exp, _ := renderMore(t, name)
		if len(exp.Files) != len(moreHosts) {
			t.Fatalf("%s wrote %d files", name, len(exp.Files))
		}
		if exp.Files[1].Name != "app"+ext {
			t.Errorf("%s names %q", name, exp.Files[1].Name)
		}
	}
	if got := winscpName("web/my host"); got != "web/my%20host" {
		t.Errorf("winscpName = %q", got)
	}
	if got := ansibleGroup("9-lives.prd"); got != "g_9_lives_prd" {
		t.Errorf("ansibleGroup = %q", got)
	}
}

func tabbyID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}
