package web

import (
	"path"
	"regexp"
	"strings"
	"testing"
)

// The navs set their type in Inter, self-hosted: every font app.css names in
// an @font-face loads, by the relative URL it gives, as font/woff2 from the
// same fingerprint app.css is served under (loadAssets' shared hash), and the
// navbar's font-family names it ahead of Tabler's own stack.
func TestTheNavFontsLoadFromAppCSS(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	page := c.get("/").Body.String()
	m := regexp.MustCompile(`<link rel="stylesheet" href="([^"]*/app\.css)">`).FindStringSubmatch(page)
	if m == nil {
		t.Fatal("the page links no app.css")
	}
	res := c.get(m[1])
	if res.Code != 200 {
		t.Fatalf("%s: %d", m[1], res.Code)
	}
	css := res.Body.String()
	if !regexp.MustCompile(`(?s)\.navbar \{[^}]*font-family: "Inter Variable", var\(--tblr-font-sans-serif\)`).MatchString(css) {
		t.Error("the navbar's font-family does not put Inter ahead of Tabler's stack")
	}
	urls := regexp.MustCompile(`url\("([^"]+\.woff2)"\)`).FindAllStringSubmatch(css, -1)
	seen := map[string]bool{}
	for _, u := range urls {
		if seen[u[1]] {
			continue
		}
		seen[u[1]] = true
		font := path.Join(path.Dir(m[1]), u[1])
		r := c.get(font)
		if r.Code != 200 || r.Header().Get("Content-Type") != "font/woff2" || !strings.HasPrefix(r.Body.String(), "wOF2") {
			t.Errorf("%s: %d %q; a font app.css names does not load", font, r.Code, r.Header().Get("Content-Type"))
		}
	}
	if len(seen) != 2 {
		t.Fatalf("app.css names %d fonts, want 2 (Inter Latin and Latin Extended)", len(seen))
	}
}
