package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// A preview, a review or a tick answers a form. Asked for again by its
// address - a reload, a restart reloading open pages into a new build, Back -
// it used to fall through to "Not found". It now says why there is nothing to
// show, and that nothing was changed.
func TestAFormAnswerAskedForAgainSaysWhy(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, path := range []string{"/changes/preview", "/changes/apply", "/jobs/review", "/jobs", "/checklist/tick", "/checklist/revoking"} {
		rec := c.get(path)
		if rec.Code != http.StatusGone || !strings.Contains(rec.Body.String(), "This page answered a form") {
			t.Errorf("GET %s: %d, want 410 and why\n%s", path, rec.Code, shorten(rec.Body.String(), 200))
		}
	}
}

// A page that answers a form names the page the form came from, so a reload -
// above all the one a server restart makes - goes back there rather than
// asking for the answer's own address.
func TestAFormAnswerReloadsToThePageItCameFrom(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	preview := func(referer string) string {
		sess := c.s.sessions.get(c.cookie.Value)
		f := addHost("reloadme")
		f.Set(csrfField, sess.csrf)
		r := httptest.NewRequest(http.MethodPost, "http://"+testHost+"/changes/preview", strings.NewReader(f.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "http://"+testHost)
		if referer != "" {
			r.Header.Set("Referer", referer)
		}
		r.AddCookie(c.cookie)
		return do(s, r).Body.String()
	}
	meta := regexp.MustCompile(`<meta name="authama-reload-to" content="([^"]*)">`)
	for _, tc := range []struct{ referer, want string }{
		{"http://" + testHost + "/hosts?profile=work", "/hosts?profile=work"},
		{"", "/"},
		{"https://elsewhere.example/hosts", "/"},
	} {
		m := meta.FindStringSubmatch(preview(tc.referer))
		if m == nil {
			t.Errorf("referer %q: the preview names no page to reload to", tc.referer)
			continue
		}
		if got := strings.ReplaceAll(m[1], "&amp;", "&"); got != tc.want {
			t.Errorf("referer %q: reloads to %q, want %q", tc.referer, got, tc.want)
		}
	}
	if strings.Contains(c.get("/hosts").Body.String(), "authama-reload-to") {
		t.Error("an ordinary page names somewhere else to reload to")
	}
}

// hrefRe is every link a page renders.
var hrefRe = regexp.MustCompile(`href="([^"#]*)(?:#[^"]*)?"`)

// Every page the menu and the palette list, and every link on those pages,
// opens: no menu entry, button or row link leads to "Not found" or an error.
func TestEveryLinkOnEveryPageOpens(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	pages := map[string]bool{}
	for _, it := range paletteItems(false) {
		p, _, _ := strings.Cut(it.Href, "#")
		if p != "" && !strings.HasPrefix(p, "/exports/") {
			pages[p] = true
		}
	}
	if len(pages) < 20 {
		t.Fatalf("only %d pages to start from; the palette is not being read", len(pages))
	}
	links := map[string]string{}
	for p := range pages {
		rec := c.get(p)
		if rec.Code != http.StatusOK {
			t.Errorf("page %s: %d", p, rec.Code)
			continue
		}
		for _, m := range hrefRe.FindAllStringSubmatch(rec.Body.String(), -1) {
			h := strings.ReplaceAll(m[1], "&amp;", "&")
			u, err := url.Parse(h)
			if h == "" || err != nil || u.Host != "" || u.Scheme != "" || !strings.HasPrefix(u.Path, "/") ||
				strings.HasPrefix(u.Path, "/assets/") || strings.HasPrefix(u.Path, "/exports/") {
				continue // the page itself, another site, a vendored asset, or a download
			}
			if _, seen := links[h]; !seen {
				links[h] = p
			}
		}
	}
	if len(links) < 35 { // 41 distinct links measured 2026-11-11
		t.Fatalf("followed only %d links; the crawl is not reading the pages", len(links))
	}
	for h, from := range links {
		if rec := c.get(h); rec.Code == http.StatusNotFound || rec.Code >= 500 {
			t.Errorf("%s links to %s, which answers %d", from, h, rec.Code)
		}
	}
}
