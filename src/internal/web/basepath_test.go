package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestCleanBasePath(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "/": "", " / ": "", "/authama": "/authama", "/authama/": "/authama",
		"/tools/authama": "/tools/authama", "/a.b_c~d-e": "/a.b_c~d-e",
	} {
		got, err := CleanBasePath(in)
		if err != nil || got != want {
			t.Errorf("CleanBasePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"authama", "/../x", "/a/./b", "//x", "/a//b", "/a b", "/a?b", "/a#b", "/a%2F", `/a"b`, "/<x>", "/-x"} {
		if got, err := CleanBasePath(bad); err == nil {
			t.Errorf("CleanBasePath(%q) = %q, want a refusal", bad, got)
		}
	}
}

// basePathAttr is any attribute whose value is a path on this server.
var basePathAttr = regexp.MustCompile(`\b(href|action|src|formaction|data-[a-z-]+)="(/[^"]*)"`)

// Served under /authama, every page reachable from the overview links, posts
// and loads only paths under /authama/: a single link or form that escapes it
// would 404 behind a reverse proxy. The crawl follows every page link it
// finds, so a page added later is covered without being listed here.
func TestEveryLinkStaysUnderTheBasePath(t *testing.T) {
	p, _ := fixtureHome(t)
	s, err := New(Options{
		Engine:       AppEngine{P: p},
		Token:        testToken,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil),
		Version:      "test",
		BasePath:     "/authama/",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Signing in redirects under the base path and scopes the cookie to it.
	rec := do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+"/authama/?token="+testToken, nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/authama/" {
		t.Fatalf("token exchange under the base path: %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	var c *client
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == SessionCookie {
			if ck.Path != "/authama/" {
				t.Errorf("session cookie path = %q, want /authama/", ck.Path)
			}
			c = &client{s: s, cookie: ck}
		}
	}
	if c == nil {
		t.Fatal("no session cookie")
	}

	// The root and the bare base path lead to the UI; other paths are not it.
	for path, want := range map[string]int{"/": http.StatusFound, "/authama": http.StatusFound, "/hosts": http.StatusNotFound} {
		if got := c.get(path).Code; got != want {
			t.Errorf("GET %s = %d, want %d", path, got, want)
		}
	}

	seen := map[string]bool{}
	queue := []string{"/authama/"}
	pages, attrs := 0, 0
	for len(queue) > 0 && pages < 300 {
		path := queue[0]
		queue = queue[1:]
		if seen[path] {
			continue
		}
		seen[path] = true
		res := c.get(path)
		if res.Code != http.StatusOK || !strings.HasPrefix(res.Header().Get("Content-Type"), "text/html") {
			continue
		}
		pages++
		for _, m := range basePathAttr.FindAllStringSubmatch(res.Body.String(), -1) {
			attr, val := m[1], m[2]
			// data-base holds the base path itself; data-copy is text put on
			// the clipboard (a file path, say), not a link.
			if strings.HasPrefix(val, "//") || attr == "data-base" || attr == "data-copy" {
				continue
			}
			attrs++
			if !strings.HasPrefix(val, "/authama/") {
				t.Errorf("%s: %s=%q escapes the base path", path, attr, val)
				continue
			}
			if attr == "href" && crawlable(val) {
				queue = append(queue, strings.SplitN(val, "#", 2)[0])
			}
		}
	}
	if pages < 15 || attrs < 150 {
		t.Fatalf("crawled %d pages and %d paths; the crawl has stopped finding the UI", pages, attrs)
	}
}

// crawlable reports whether a link is a page to follow: not an asset, a
// download or an event stream that never ends.
func crawlable(p string) bool {
	return !strings.Contains(p, "/assets/") && !strings.Contains(p, "/exports/") &&
		!strings.HasSuffix(strings.SplitN(p, "?", 2)[0], "/events") || strings.HasPrefix(p, "/authama/events")
}

// A Location the handlers set is put back under the base path; one that is
// already absolute, or scheme-relative, is left alone.
func TestARedirectIsPutBackUnderTheBasePath(t *testing.T) {
	for loc, want := range map[string]string{
		"/hosts": "/authama/hosts", "https://example.com/x": "https://example.com/x", "//h/x": "//h/x",
	} {
		h := withBasePath("/authama", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/x" {
				t.Errorf("the handler saw %q, want the base path taken off", r.URL.Path)
			}
			http.Redirect(w, r, loc, http.StatusSeeOther)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/authama/x", nil))
		if got := rec.Header().Get("Location"); got != want {
			t.Errorf("Location %q became %q, want %q", loc, got, want)
		}
	}
}
