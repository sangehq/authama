package web

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The Help link opens Tool integrations, and each tool's card lists the hosts
// that tool's export writes and the ones it leaves out, read from the export
// itself: a host reached through Cloudflare Access is left out of Termius,
// whose CSV cannot run a proxy, while its WARP route goes in, and OpenSSH
// gets both.
func TestToolIntegrationsListWhatEachExportWrites(t *testing.T) {
	s, p, _ := newTestServer(t)
	m := `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","hosts":[
	    {"alias":"web","hostname":"ssh-web.example.com","user":"deploy","proxy":{"kind":"cloudflared"},
	     "variants":[{"name":"warp","hostname":"10.0.0.3","proxy":{"kind":"none"}}]}]}}}`
	if err := os.WriteFile(p.Manifest(), []byte(m), 0o600); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	if home := c.get("/").Body.String(); !strings.Contains(home, `href="/integrations"`) || !strings.Contains(home, `<span class="nav-link-title">Help</span>`) {
		t.Fatal("the main menu has no Help link to /integrations")
	}
	rec := c.get("/integrations")
	if rec.Code != 200 {
		t.Fatalf("/integrations: %d", rec.Code)
	}
	body := rec.Body.String()
	cards := regexp.MustCompile(`(?s)<div class="card h-100" id="tool-([a-z0-9-]+)">(.*?)<div class="card-footer`).FindAllStringSubmatch(body, -1)
	if len(cards) < 15 {
		t.Fatalf("read %d tool cards, want at least 15 (OpenSSH, Termius and every client export)", len(cards))
	}
	card := map[string]string{}
	for _, c := range cards {
		card[c[1]] = c[2]
	}
	gets := func(tool, alias string) bool {
		i := strings.Index(card[tool], "Hosts it gets")
		j := strings.Index(card[tool], "Left out")
		in := card[tool]
		if i < 0 {
			return false
		}
		if j > i {
			in = in[i:j]
		} else {
			in = in[i:]
		}
		return strings.Contains(in, "<code>"+alias+"</code>")
	}
	if !gets("openssh", "web") || !gets("openssh", "warp-web") {
		t.Errorf("OpenSSH should get web and warp-web:\n%s", card["openssh"])
	}
	if gets("termius", "web") || !gets("termius", "warp-web") {
		t.Errorf("Termius should get warp-web and leave web out:\n%s", card["termius"])
	}
	if !strings.Contains(card["termius"], "<code>web</code>: reached through cloudflared") {
		t.Errorf("Termius's card does not say why web is left out:\n%s", card["termius"])
	}
	if !strings.Contains(body, `id="int-hosts">1<`) || !strings.Contains(body, `id="int-routes">1<`) {
		t.Error("the totals are not 1 host and 1 route")
	}
}
