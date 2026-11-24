package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var (
	navTileRe  = regexp.MustCompile(`class="avatar avatar-sm nav-tile bg-([a-z]+)-lt" data-kind="([a-z]+)"`)
	navGlyphRe = regexp.MustCompile(`class="nav-link-icon nav-glyph text-([a-z]+)\b`)
)

// navOwnerColours is the colour the owner chose for each kind of task and for the
// menus whose colour he named (2026-11-16); nav-redesign.md holds the table.
var navOwnerColours = map[string]string{
	"create": "green", "change": "blue", "revoke": "orange", "delete": "red",
	"health": "teal", "security": "purple", "history": "yellow", "settings": "secondary",
}

var navTaskKinds = map[string]bool{"create": true, "change": true, "revoke": true, "delete": true}

// Every entry in the header's menus (the main menu's dropdowns, New and the
// user menu) shows its icon on a colour tile, and the colour follows what the
// entry is: one kind has one colour everywhere, a task's colour says what it
// does, each menu's pages share one colour no other menu's pages use, a red
// (destructive) entry is a delete, and each toggle's icon is its menu's colour.
func TestNavColoursFollowTheMapping(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := signIn(t, s).get("/").Body.String()
	entries := navEntries(t, body)
	top, main := headerParts(t, body)
	header := top + main

	kindColour := map[string]map[string]bool{}
	pageColours := map[string]map[string]bool{} // menu -> colours of its pages
	tiles := 0
	for _, e := range entries {
		if e.Toggle || e.Menu == "main" {
			continue
		}
		m := navTileRe.FindStringSubmatch(e.inner)
		if m == nil {
			t.Errorf("%s › %s has no colour tile", e.Menu, e.Label)
			continue
		}
		tiles++
		colour, kind := m[1], m[2]
		if want, ok := kindColours[kind]; !ok || want != colour {
			t.Errorf("%s › %s is kind %s in %s; the mapping says %q", e.Menu, e.Label, kind, colour, want)
		}
		if kindColour[kind] == nil {
			kindColour[kind] = map[string]bool{}
		}
		kindColour[kind][colour] = true
		if want := navOwnerColours[kind]; want != "" && want != colour {
			t.Errorf("%s › %s is a %s entry in %s, want %s", e.Menu, e.Label, kind, colour, want)
		}
		if strings.Contains(e.attrs, "text-danger") && kind != "delete" {
			t.Errorf("%s › %s is red (text-danger) but its kind is %s, not delete", e.Menu, e.Label, kind)
		}
		// Export / Import, beside New, repeats History's export pages in
		// History's colour, as New repeats the creates: it owns no colour.
		// Help holds reference pages in the user menu's personal colour, so
		// it owns none either.
		if !navTaskKinds[kind] && e.Menu != "user" && e.Menu != "system" && e.Menu != "transfer" && e.Menu != "help" {
			if pageColours[e.Menu] == nil {
				pageColours[e.Menu] = map[string]bool{}
			}
			pageColours[e.Menu][colour] = true
		}
	}
	// The floor: 6 + 6 SSH, 3 Tokens, 3 Security, 4 History, 5 System,
	// 3 New, 4 user: 34. A reader that stopped matching reports fewer.
	if tiles < 34 {
		t.Fatalf("read %d colour tiles, want at least 34; the reading is not matching the menus", tiles)
	}
	for kind, cs := range kindColour {
		if len(cs) != 1 {
			t.Errorf("kind %s shows %d colours: %v", kind, len(cs), cs)
		}
	}
	for kind := range navOwnerColours {
		if kindColour[kind] == nil {
			t.Errorf("no menu entry of kind %s was read; the reading is not matching", kind)
		}
	}
	owner := map[string]string{}
	for menu, cs := range pageColours {
		if len(cs) != 1 {
			t.Errorf("the %s menu's pages show %d colours, want one: %v", menu, len(cs), cs)
		}
		for c := range cs {
			if other, ok := owner[c]; ok {
				t.Errorf("the %s and %s menus' pages share the colour %s", menu, other, c)
			}
			owner[c] = menu
		}
	}
	if len(pageColours) != 4 {
		t.Errorf("read the pages of %d menus, want 4 (SSH, Tokens, Security, History)", len(pageColours))
	}

	// Each toggle's icon is its menu's colour: the pages' colour, and for
	// System, which holds two kinds, Health's.
	glyphs := map[string]string{"ssh": kindColours["ssh"], "tokens": kindColours["tokens"],
		"security": kindColours["security"], "history": kindColours["history"], "system": kindColours["health"],
		"help": kindColours["personal"]}
	for menu, want := range glyphs {
		i := strings.Index(header, `id="navbar-`+menu+`"`)
		if i < 0 {
			t.Errorf("no %s toggle", menu)
			continue
		}
		end := strings.Index(header[i:], "</a>")
		g := navGlyphRe.FindStringSubmatch(header[i : i+end])
		if g == nil || g[1] != want {
			t.Errorf("the %s toggle's icon colour = %v, want text-%s", menu, g, want)
		}
	}
}

// A menu entry's description fits two lines at the menu's width: about 60
// characters (nav-redesign.md, "Text"). A longer one wraps to three.
func TestMenuDescriptionsFitTwoLines(t *testing.T) {
	n := 0
	for key := range navKinds {
		tip, ok := pageTips[key]
		if !ok {
			t.Errorf("menu entry %s has no description in pageTips", key)
			continue
		}
		n++
		if len(tip) > 62 {
			t.Errorf("%s's description is %d characters, over 62: %q", key, len(tip), tip)
		}
	}
	if n < 25 {
		t.Fatalf("checked %d descriptions, want at least 25", n)
	}
}

// Overview, chosen from any other page, opens the dashboard: the page at the
// root of the server, or of its base path, whose Overview link is current.
func TestOverviewLeadsToTheDashboard(t *testing.T) {
	for _, base := range []string{"", "/authama"} {
		t.Run("base="+base, func(t *testing.T) {
			p, _ := fixtureHome(t)
			s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken,
				AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Version: "test", BasePath: base + "/"})
			if err != nil {
				t.Fatal(err)
			}
			rec := do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+base+"/?token="+NewLink(s).Token, nil))
			var c *client
			for _, ck := range rec.Result().Cookies() {
				if ck.Name == SessionCookie {
					c = &client{s: s, cookie: ck}
				}
			}
			if c == nil {
				t.Fatalf("signing in under %q: %d, no session cookie", base, rec.Code)
			}
			for _, from := range []string{"/hosts", "/keys", "/settings", "/about"} {
				res := c.get(base + from)
				if res.Code != http.StatusOK {
					t.Fatalf("GET %s%s = %d", base, from, res.Code)
				}
				href := ""
				for _, e := range navEntries(t, res.Body.String()) {
					if e.Menu == "main" && e.Label == "Overview" {
						href = e.Href
					}
				}
				if href != base+"/" {
					t.Errorf("from %s the Overview link is %q, want %q", from, href, base+"/")
					continue
				}
				dash := c.get(href)
				if dash.Code != http.StatusOK {
					t.Fatalf("Overview (%s) from %s = %d, want 200", href, from, dash.Code)
				}
				b := dash.Body.String()
				if !strings.Contains(b, "<title>Overview ·") {
					t.Errorf("Overview from %s opened a page that is not the dashboard", from)
				}
				_, main := headerParts(t, b)
				if !regexp.MustCompile(`<a class="nav-link" href="` + regexp.QuoteMeta(href) + `" aria-current="page"`).MatchString(main) {
					t.Errorf("on the page Overview opened from %s, Overview is not the current page", from)
				}
			}
		})
	}
}
