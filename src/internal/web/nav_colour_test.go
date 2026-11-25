package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

var (
	navTileRe = regexp.MustCompile(`class="icon dropdown-item-icon nav-icon" data-kind="([a-z]+)"`)
)

// navOwnerColours is the colour the owner chose for each kind of task and for the
// menus whose colour he named (2026-11-16); nav-redesign.md holds the table.
var navOwnerColours = map[string]string{
	"create": "green", "change": "blue", "revoke": "orange", "delete": "red",
	"health": "teal", "security": "purple", "history": "yellow", "settings": "secondary",
}

// Every entry in the header's menus (the main menu's dropdowns, New and the
// user menu) shows Tabler's plain icon (navIcon), no colour tile (owner,
// 2026-11-24: "maintain the same minimal icon design as ... in the tabler
// css"), and says what kind of entry it is: one the mapping knows, a red
// (destructive) entry is a delete, and each of the owner's task kinds is
// read somewhere. Each menu's toggle shows its icon in Tabler's own grey.
func TestNavColoursFollowTheMapping(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := signIn(t, s).get("/").Body.String()
	entries := navEntries(t, body)
	top, main := headerParts(t, body)
	header := top + main
	if strings.Contains(header, "nav-tile") || regexp.MustCompile(`-lt" data-kind=`).MatchString(header) {
		t.Error("a menu entry still shows a colour tile; entries show Tabler's plain dropdown-item-icon")
	}

	kinds := map[string]bool{}
	icons := 0
	for _, e := range entries {
		if e.Toggle || e.Menu == "main" || e.Label == "Running as" {
			continue // Running as is the account header: text, no icon
		}
		m := navTileRe.FindStringSubmatch(e.inner)
		if m == nil {
			t.Errorf("%s › %s has no Tabler dropdown-item-icon", e.Menu, e.Label)
			continue
		}
		icons++
		kind := m[1]
		if _, ok := kindColours[kind]; !ok {
			t.Errorf("%s › %s is kind %s, which the mapping does not know", e.Menu, e.Label, kind)
		}
		kinds[kind] = true
		if strings.Contains(e.attrs, "text-danger") && kind != "delete" {
			t.Errorf("%s › %s is red (text-danger) but its kind is %s, not delete", e.Menu, e.Label, kind)
		}
	}
	// The floor: 7 + 4 + 6 SSH, 3 Tokens, 3 Security, 5 History, 8 System,
	// 3 New, 6 user: 45. A reader that stopped matching reports fewer.
	if icons < 40 {
		t.Fatalf("read %d menu icons, want at least 40; the reading is not matching the menus", icons)
	}
	for kind := range navOwnerColours {
		if !kinds[kind] {
			t.Errorf("no menu entry of kind %s was read; the reading is not matching", kind)
		}
	}

	// Each toggle's icon is Tabler's own grey: no colour class on it, the
	// same for every menu (owner, 2026-11-24: "maintain same nav color as
	// tabler does").
	toggles := 0
	for _, menu := range []string{"ssh", "tokens", "security", "history", "system", "help"} {
		i := strings.Index(header, `id="navbar-`+menu+`"`)
		if i < 0 {
			t.Errorf("no %s toggle", menu)
			continue
		}
		toggles++
		end := strings.Index(header[i:], "</a>")
		if g := regexp.MustCompile(`nav-link-icon[^"]*\btext-[a-z]+`).FindString(header[i : i+end]); g != "" {
			t.Errorf("the %s toggle's icon is coloured (%s); Tabler draws it grey", menu, g)
		}
	}
	if toggles != 6 {
		t.Fatalf("read %d toggles, want 6", toggles)
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
