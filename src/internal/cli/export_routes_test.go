package cli

import (
	"encoding/json"
	"testing"
)

// A host's extra names, how ssh reaches it and its other routes can be
// exported (aliases, route, routes), without changing hosts.csv's default
// columns.
func TestExportHostsCarriesAliasesAndRoutes(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "web", "-H", "ssh-web.example", "-u", "deploy", "--proxy", "cloudflared", "--alias", "web-cf")
	run(t, "ssh", "host", "variant", "set", "work", "web", "warp", "-H", "10.116.0.3", "--proxy", "none")
	out := run(t, "export", "hosts", "--columns", "alias,aliases,route,routes", "--format", "json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: %v", rows)
	}
	r := rows[0]
	if r["route"] != "cloudflared" {
		t.Errorf("route = %v, want cloudflared", r["route"])
	}
	for field, want := range map[string]string{"aliases": "web-cf", "routes": "warp-web"} {
		list, ok := r[field].([]any)
		if !ok || len(list) != 1 || list[0] != want {
			t.Errorf("%s = %v, want [%s]", field, r[field], want)
		}
	}
	if def := run(t, "export", "hosts"); def != run(t, "ssh", "list", "--csv") {
		t.Error("the default hosts export no longer matches ssh list --csv")
	}
}
