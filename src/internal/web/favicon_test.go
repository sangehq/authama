package web

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/brand"
)

// Every page names an embedded SVG favicon, so no load asks for /favicon.ico,
// and that favicon is the header's logo mark: a primary rounded square with a
// white glyph, so the tab and the header read as one product.
func TestEveryPageNamesTheLogoMarkFavicon(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	pages := []string{"/", "/keys", "/hosts", "/vms", "/about", "/settings"}
	link := regexp.MustCompile(`<link rel="icon" type="image/svg\+xml" href="([^"]+/favicon\.svg)">`)
	var href string
	for _, p := range pages {
		body := c.get(p).Body.String()
		m := link.FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("%s names no SVG favicon", p)
		}
		href = m[1]
		for _, want := range []string{
			`<img class="navbar-brand-mark" src="` + href + `"`,
			`<span class="navbar-brand-text d-none d-lg-inline">` + brand.Name + `</span>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the header has no %q", p, want)
			}
		}
	}
	rec := c.get(href)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s answered %d", href, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("%s is served as %q", href, ct)
	}
	svg := rec.Body.String()
	for _, want := range []string{`viewBox="0 0 32 32"`, `rx="7"`, `fill="#066fd1"`, `stroke="#fff"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("the favicon is not the logo mark: no %s", want)
		}
	}
}

// The sign-in page, which has no session, and the not-found page still name
// the favicon.
func TestTheSignInAndNotFoundPagesNameTheFavicon(t *testing.T) {
	s, _, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Host = "127.0.0.1"
	if body := do(s, req).Body.String(); !strings.Contains(body, `type="image/svg+xml" href="`) {
		t.Errorf("a page without a session names no favicon:\n%.400s", body)
	}
	rec := signIn(t, s).get("/no-such-page")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/no-such-page answered %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `type="image/svg+xml" href="`) {
		t.Error("the not-found page names no favicon")
	}
}
