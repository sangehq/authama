package query

import (
	"encoding/json"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
)

// routedManifest is a host with an extra name, a Cloudflare Access proxy and
// a WARP variant: the shape the Hosts list used to flatten into one row that
// showed none of the three.
const routedManifest = `{
  "version": 1,
  "defaults": {"key_type": "ed25519"},
  "profiles": {
    "org": {"key_scope": "per_service", "hosts": [
      {"alias": "web", "hostname": "ssh-web.example.com", "user": "deploy",
       "proxy": {"kind": "cloudflared"}, "aliases": ["web-cf"],
       "variants": [{"name": "warp", "hostname": "10.0.0.3", "proxy": {"kind": "none"}}]},
      {"alias": "plain", "hostname": "203.0.113.9", "user": "deploy"},
      {"alias": "jumped", "hostname": "10.0.0.9", "user": "deploy", "raw_options": {"ProxyJump": "bastion"}},
      {"alias": "devbox", "hostname": "devbox", "user": "dev", "vm": {"hypervisor": "incus", "name": "devbox"}}
    ]}
  }
}`

func routedRows(t *testing.T) map[string]HostRow {
	t.Helper()
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(routedManifest), &m); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	groups, err := New(&m, inventory.New(), "").Groups("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]HostRow{}
	for _, g := range groups {
		for _, r := range g.Rows {
			rows[r.Alias] = r
		}
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4 (one per manifest host; variants are not rows)", len(rows))
	}
	return rows
}

func TestHostRowCarriesAliasesRouteAndVariants(t *testing.T) {
	web := routedRows(t)["web"]
	if len(web.Aliases) != 1 || web.Aliases[0] != "web-cf" {
		t.Fatalf("Aliases = %v, want [web-cf]", web.Aliases)
	}
	if web.Route != "cloudflared" {
		t.Fatalf("Route = %q, want cloudflared", web.Route)
	}
	if len(web.Variants) != 1 {
		t.Fatalf("Variants = %+v, want one", web.Variants)
	}
	v := web.Variants[0]
	if v.Alias != "warp-web" || v.Hostname != "10.0.0.3" || v.Route != "direct" {
		t.Fatalf("variant = %+v, want warp-web at 10.0.0.3, route direct", v)
	}
}

func TestHostRowRouteForPlainAndJumpHosts(t *testing.T) {
	rows := routedRows(t)
	if r := rows["plain"]; r.Route != "" || len(r.Aliases) != 0 || len(r.Variants) != 0 {
		t.Fatalf("plain host = %+v, want no route, aliases or variants", r)
	}
	if r := rows["jumped"]; r.Route != "jump bastion" {
		t.Fatalf("jumped Route = %q, want %q", r.Route, "jump bastion")
	}
	if r := rows["devbox"]; r.Route != "vm" {
		t.Fatalf("devbox Route = %q, want vm", r.Route)
	}
}
