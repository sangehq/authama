package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Sim's simbi blocks, written by hand into ~/.ssh/config, import as two
// hosts with a second name each, a Cloudflare Access proxy and their
// sections, and two WARP hosts with ProxyCommand none and a timeout; the
// blocks move into the managed config, which renders them back as written
// (plus the UserKnownHostsFile line every managed host carries).
func TestImportTakesInAliasesProxiesAndSections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	block, err := os.ReadFile("../../core/renderer/testdata/routes/simbi.block")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"simtabi_simbi-prd-web-00-ed25519", "simtabi_simbi-prd-db-00-ed25519"} {
		mintKey(t, filepath.Join(ssh, "profiles", "orgs", "simtabi", k))
	}
	cfgPath := filepath.Join(ssh, "config")
	if err := os.WriteFile(cfgPath, block, 0o600); err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{SSHDir: ssh, ConfigDir: t.TempDir()}
	res, err := New(p, false).Run(cfgPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 0 || len(res.Kept) != 0 {
		t.Fatalf("skipped %v, kept %v", res.Skipped, res.Kept)
	}
	m := mustLoad(t, p)
	web, _ := hostIn(m, "simbi-prd-web-00")
	if strings.Join(web.Aliases, ",") != "simbi-web-cf" || web.Proxy == nil || web.Proxy.Kind != "cloudflared" ||
		!strings.HasPrefix(web.Section, "simbi prod: Cloudflare Access") {
		t.Errorf("web host: %+v", web)
	}
	if h, _ := hostIn(m, "simbi-web-cf"); h.Alias != "simbi-prd-web-00" {
		t.Errorf("simbi-web-cf is not a name of simbi-prd-web-00: %+v", h)
	}
	warp, _ := hostIn(m, "warp-simbi-prd-db-00")
	if warp.Proxy == nil || warp.Proxy.Kind != "none" || warp.RawOptions.Get("ConnectTimeout") != "10" {
		t.Errorf("warp host: %+v", warp)
	}

	prev := renderer.CloudflaredPath
	renderer.CloudflaredPath = func() string { return "/opt/homebrew/bin/cloudflared" }
	t.Cleanup(func() { renderer.CloudflaredPath = prev })
	got, err := renderer.RenderCategoryConfig(m, "orgs")
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.ReplaceAll(got, "    UserKnownHostsFile ~/.ssh/known_hosts\n", "")
	if !strings.Contains(stripped, string(block)) {
		t.Errorf("the imported blocks do not render back as written:\n%s\nwant\n%s", stripped, block)
	}
	if b, _ := os.ReadFile(cfgPath); strings.Contains(string(b), "Host simbi-prd-web-00") {
		t.Errorf("the hand-written block stayed in ~/.ssh/config:\n%s", b)
	}
}
