package renderer

import (
	"strings"
	"testing"
)

// routedManifest is a host reached through Cloudflare Access with a WARP
// route beside it (ProxyCommand none), and a plain host.
const routedManifest = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
  "org":{"key_scope":"per_service","hosts":[
    {"alias":"web","hostname":"ssh-web.example.com","user":"deploy","proxy":{"kind":"cloudflared"},
     "variants":[{"name":"warp","hostname":"10.0.0.3","proxy":{"kind":"none"}}]},
    {"alias":"plain","hostname":"203.0.113.9","user":"deploy"}]}}}`

// A selection that keeps a host keeps the routes rendered beside it, as the
// whole export does: the Termius page and `export hosts --profile` used to
// write web's WARP route nowhere, while the unfiltered export wrote it.
func TestClientHostsKeepsASelectedHostsRoutes(t *testing.T) {
	m := mustManifest(t, routedManifest)
	hosts, skipped, err := Source{M: m, P: testExportPaths}.ClientHosts([]string{"web", "plain"}, "Termius", false, false)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range hosts {
		got = append(got, h.Alias+"@"+h.Hostname)
	}
	if strings.Join(got, " ") != "plain@203.0.113.9 warp-web@10.0.0.3" {
		t.Fatalf("hosts = %v, want plain then warp-web (web's route goes direct, so Termius can use it)", got)
	}
	if len(skipped) != 1 || !strings.HasPrefix(skipped[0], "web: ") {
		t.Fatalf("skipped = %v, want web alone (it needs cloudflared)", skipped)
	}
	// Choosing only plain brings no route of web's.
	hosts, _, err = Source{M: m, P: testExportPaths}.ClientHosts([]string{"plain"}, "Termius", false, false)
	if err != nil || len(hosts) != 1 || hosts[0].Alias != "plain" {
		t.Fatalf("plain alone: %v, %v", hosts, err)
	}
}

func TestFlatConfigOfAHostKeepsItsRoutes(t *testing.T) {
	m := mustManifest(t, routedManifest)
	out, err := RenderFlatConfigOf(m, testExportPaths, false, []string{"web"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Host warp-web\n") {
		t.Fatalf("the flat config of web lacks its warp-web route:\n%s", out)
	}
	if strings.Contains(out, "Host plain\n") {
		t.Fatal("the flat config of web holds plain, which was not chosen")
	}
}
