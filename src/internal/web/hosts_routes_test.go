package web

import (
	"os"
	"strings"
	"testing"
)

// The Hosts list shows a host's extra names, how ssh reaches it and each of
// its other routes. It used to show one row per host and none of the three,
// so warp-<alias> and an extra name existed in ~/.ssh/config and nowhere on
// the page.
func TestHostsListShowsAliasesRoutesAndVariants(t *testing.T) {
	s, p, _ := newTestServer(t)
	m := `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","hosts":[
	    {"alias":"gh","hostname":"github.com","user":"git"},
	    {"alias":"web","hostname":"ssh-web.example.com","user":"deploy","proxy":{"kind":"cloudflared"},"aliases":["web-cf"],
	     "variants":[{"name":"warp","hostname":"10.0.0.3","proxy":{"kind":"none"}}]}]}}}`
	if err := os.WriteFile(p.Manifest(), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	body := signIn(t, s).get("/hosts").Body.String()
	for _, want := range []string{
		`<code>web-cf</code>`,         // the extra name
		`>cloudflared</span>`,         // the primary route
		`data-variant-of="web"`,       // the variant's row
		`<code>warp-web</code>`,       // its alias
		`10.0.0.3 <span class="badge`, // its address and route badge
		`>direct</span>`,              // ProxyCommand none
		`data-copy="ssh warp-web"`,    // its copy action
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/hosts lacks %q", want)
		}
	}
	if n := strings.Count(body, `class="host-variant"`); n != 1 {
		t.Fatalf("%d variant rows, want 1", n)
	}
}
