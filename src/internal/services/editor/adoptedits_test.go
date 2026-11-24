package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/util/paths"
)

const simtabiManifest = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{"simtabi":{"category":"orgs","hosts":[
	{"alias":"oribi-web","hostname":"203.0.113.11","user":"ploi"},
	{"alias":"oribi-db-maria","hostname":"203.0.113.12","user":"ploi"},
	{"alias":"oribi-db-psql","hostname":"203.0.113.13","user":"ploi"}]}}}`

// simtabiHome is a home whose orgs file authama rendered, with the record.
func simtabiHome(t *testing.T) (*Editor, paths.Paths, string) {
	t.Helper()
	prev := renderer.CloudflaredPath
	renderer.CloudflaredPath = func() string { return "/opt/homebrew/bin/cloudflared" }
	t.Cleanup(func() { renderer.CloudflaredPath = prev })
	cfg := t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg, "manifest.json"), []byte(simtabiManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{SSHDir: filepath.Join(t.TempDir(), ".ssh"), ConfigDir: cfg}
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := configsvc.New(p.SSHDir, reload(t, p), false).WithState(p.StateDir()).Write(false); err != nil {
		t.Fatal(err)
	}
	return New(p), p, filepath.Join(p.SSHDir, "profiles", "orgs", "config")
}

// Sim pasted his simbi blocks into the managed orgs file. keep-edits takes
// them into the manifest as two hosts with a second name each and their WARP
// route variants, and the render that follows reproduces his file - with the
// UserKnownHostsFile line authama writes for every host.
func TestAdoptEditsTakesSimsPastedBlocksIn(t *testing.T) {
	ed, p, orgs := simtabiHome(t)
	block, err := os.ReadFile("../../core/renderer/testdata/routes/simbi.block")
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := os.ReadFile(orgs)
	if err := os.WriteFile(orgs, append(cur, block...), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := func() *configsvc.Service {
		return configsvc.New(p.SSHDir, reload(t, p), false).WithState(p.StateDir())
	}
	if _, err := svc().Write(false); !configsvc.IsHandEdits(err) {
		t.Fatalf("the render must refuse first: %v", err)
	}

	dry, err := RoutesOf(ed).KeepEdits(false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Adopted) != 4 || len(reload(t, p).Profiles["simtabi"].Hosts) != 3 {
		t.Fatalf("dry run: %s", dry.Format(true))
	}
	res, err := RoutesOf(ed).KeepEdits(false, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"simtabi/simbi-prd-web-00: added host", "simtabi/simbi-prd-db-00: added host",
		"simtabi/warp-simbi-prd-web-00: added variant warp of simbi-prd-web-00",
		"simtabi/warp-simbi-prd-db-00: added variant warp of simbi-prd-db-00",
	}
	got := res.Format(false)
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	if len(res.Left) != 0 {
		t.Errorf("left: %v", res.Left)
	}
	m := reload(t, p)
	web := m.Profiles["simtabi"].Hosts[3]
	if web.Alias != "simbi-prd-web-00" || len(web.Aliases) != 1 || web.Aliases[0] != "simbi-web-cf" ||
		web.Proxy == nil || web.Proxy.Kind != manifest.ProxyCloudflared || web.KeyName != nil ||
		len(web.Variants) != 1 || web.Variants[0].Hostname != "10.116.0.3" || web.Variants[0].Proxy.Kind != manifest.ProxyNone ||
		web.Variants[0].RawOptions.Get("ConnectTimeout") != "10" {
		t.Errorf("simbi-prd-web-00 = %+v", web)
	}

	if _, err := svc().Write(false); err != nil {
		t.Fatalf("after adopting, the render goes ahead: %v", err)
	}
	after, _ := os.ReadFile(orgs)
	golden, _ := os.ReadFile("../../core/renderer/testdata/routes/orgs.config")
	if string(after) != string(golden)+"\n" { // the golden copy leaves off the file's closing blank line
		t.Errorf("rendered after adopting:\n%s\nwant\n%s", after, golden)
	}
	again, err := RoutesOf(ed).KeepEdits(false, false)
	if err != nil || len(again.Adopted) != 0 {
		t.Errorf("a second adopt finds nothing: %v %v", again, err)
	}
}

// A hand edit to a managed host - an option added, a value changed - updates
// that host; a block that cannot be taken in whole is left with the reason.
func TestAdoptEditsUpdatesAndLeavesWhatItCannotTake(t *testing.T) {
	ed, p, orgs := simtabiHome(t)
	cur, _ := os.ReadFile(orgs)
	s := strings.Replace(string(cur), "    HostName 203.0.113.11\n", "    HostName 203.0.113.21\n    ServerAliveInterval 15\n", 1)
	s += "Host stray\n    HostName stray.example\n    User root\n    IdentityFile ~/.ssh/id_ed25519\n\n" +
		"Host runs\n    HostName runs.example\n    User root\n    IdentityFile ~/.ssh/profiles/orgs/simtabi/k\n    LocalCommand touch /tmp/x\n"
	if err := os.WriteFile(orgs, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := RoutesOf(ed).KeepEdits(false, false)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Format(false)
	for _, want := range []string{"simtabi/oribi-web: updated host", "stray: its IdentityFile ~/.ssh/id_ed25519 is not in the profile's key directory",
		"runs: LocalCommand can run a command"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	h := reload(t, p).Profiles["simtabi"].Hosts[0]
	if h.Hostname != "203.0.113.21" || h.RawOptions.Get("ServerAliveInterval") != "15" {
		t.Errorf("oribi-web = %+v", h)
	}
	if len(reload(t, p).Profiles["simtabi"].Hosts) != 3 {
		t.Error("a block left out must add nothing")
	}
}
