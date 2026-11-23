package web

import (
	"html"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

// termiusHome is a home with each kind of host the Termius page treats
// differently: categorized and named by the rules, behind a ProxyJump, behind
// a ProxyCommand (left out), and git (left out unless asked for).
func termiusHome(t *testing.T) *client {
	t.Helper()
	s, p, _ := newTestServer(t)
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1,"profiles":{
		"work":{"code":"wrk","category":"clients","hosts":[
			{"alias":"wrk-prd-web-00","hostname":"203.0.113.5","user":"deploy","tags":["web","prd"]},
			{"alias":"wrk-stg-db-00","hostname":"203.0.113.6","user":"deploy","raw_options":{"ProxyJump":"wrk-prd-web-00"}},
			{"alias":"wrk-prd-cf-00","hostname":"ssh.example.com","user":"deploy","proxy":{"kind":"cloudflared"}}]},
		"solo":{"hosts":[{"alias":"box","hostname":"box.example","user":"me","port":2222},
			{"alias":"gh","hostname":"github.com","user":"git"}]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return signIn(t, s)
}

// downloadOf is the page's Download link, decoded.
func downloadOf(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`href="(/exports/run\?[^"]+)" download`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no download link:\n%s", body)
	}
	return html.UnescapeString(m[1])
}

// The page shows the CSV it downloads: by default grouped by profile, the
// ProxyCommand host and the git host left out with their reasons, and every
// group scheme offered with an example drawn from the hosts.
func TestTermiusPageShowsTheCSVItDownloads(t *testing.T) {
	c := termiusHome(t)
	rec := c.get("/termius")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("/termius: %d\n%s", rec.Code, body)
	}
	for _, want := range []string{
		"<h1 class=\"page-title\">Termius import</h1>", "3 of 5 host(s) in the file",
		"Groups,Label,Tags,Hostname/IP,Protocol,Port,Username,Password", "work,wrk-prd-web-00,", "solo,box,,box.example,ssh,2222,me,",
		"wrk-prd-cf-00: reached through a ProxyCommand", "gh: a git host",
		`value="category/profile/env"`, "<code class=\"ms-1\">clients/work/prd</code>", "(no group)",
		"authama export hosts --format termius --out termius-hosts.csv",
		"Host Chain", "<code>wrk-stg-db-00</code> through <code>wrk-prd-web-00</code>",
		"authama ssh termius keys wrk-prd-web-00 --clear-after 60s", "authama ssh termius keys --clear",
		"Keychain → New Key",
	} {
		if !strings.Contains(html.UnescapeString(body), want) {
			t.Errorf("/termius lacks %q", want)
		}
	}
	got := c.get(downloadOf(t, body))
	if got.Code != http.StatusOK || !strings.Contains(html.UnescapeString(body), got.Body.String()) {
		t.Errorf("the download (%d) is not the CSV the page shows:\n%s", got.Code, got.Body.String())
	}
	if want := c.get("/exports/termius-hosts.csv").Body.String(); got.Body.String() != want {
		t.Errorf("the default download differs from /exports/termius-hosts.csv:\n%s\n---\n%s", got.Body.String(), want)
	}
}

// Every choice the CLI has reaches the page, its download and its command:
// the group scheme, git hosts, and the filters.
func TestTermiusPageTakesEveryExportChoice(t *testing.T) {
	c := termiusHome(t)
	body := html.UnescapeString(c.get("/termius?termius_groups=category/profile/env&include_git=1&profile=work&profile=solo&env=prd&exclude=cf").Body.String())
	for _, want := range []string{
		"1 of 1 host(s) in the file", "clients/work/prd,wrk-prd-web-00,", `value="prd" checked`,
		"authama export hosts --format termius --termius-groups category/profile/env --include-git --profile work,solo --env prd --exclude cf --out termius-hosts.csv",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("filtered /termius lacks %q", want)
		}
	}
	if strings.Contains(body, "solo,box,") {
		t.Error("--env prd kept box")
	}
	dl := c.get(downloadOf(t, body)).Body.String()
	if !strings.Contains(dl, "clients/work/prd,wrk-prd-web-00,") || strings.Contains(dl, "box") {
		t.Errorf("the download ignores the choices:\n%s", dl)
	}
	git := html.UnescapeString(c.get("/termius?include_git=1&termius_groups=none").Body.String())
	if !strings.Contains(git, ",gh,,github.com,ssh,22,git,") || strings.Contains(git, "gh: a git host") {
		t.Error("include_git=1 does not keep the git host")
	}
	if rec := c.get("/termius?termius_groups=env"); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown scheme: %d", rec.Code)
	}
	if text := html.UnescapeString(c.get("/termius?include=nothing-matches").Body.String()); !strings.Contains(text, "No host goes into the file") {
		t.Error("an empty selection does not say so")
	}
}

// The keys step names each host's label and key path and the command that
// copies the key, but no private key ever reaches the page, and its form
// only reads.
func TestTermiusPageNeverCarriesAPrivateKey(t *testing.T) {
	c := termiusHome(t)
	body := c.get("/termius?include_git=1").Body.String()
	if strings.Contains(body, "PRIVATE KEY") {
		t.Error("/termius carries a private key")
	}
	if !strings.Contains(body, `<form method="get" action="/termius" class="card" id="termius-options"`) {
		t.Error("the options form is not a GET")
	}
	if !strings.Contains(body, "work_wrk-prd-web-00") && !strings.Contains(body, "wrk-prd-web-00") {
		t.Error("no key label")
	}
	if !strings.Contains(body, "This page never copies a private key") {
		t.Error("the page does not say why it does not copy the key")
	}
}

// The hosts page and the palette lead to the page.
func TestTermiusPageIsReachable(t *testing.T) {
	c := termiusHome(t)
	if !strings.Contains(c.get("/hosts").Body.String(), `href="/termius"`) {
		t.Error("the hosts page does not link the Termius import")
	}
	found := false
	for _, it := range paletteItems(true) {
		found = found || it.Href == "/termius"
	}
	if !found {
		t.Error("the palette has no Termius import")
	}
}
