package web

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// navEntry is one thing in the header's menus a person can see and choose:
// a menu's toggle, an entry in it, the Overview link, or the user menu's
// Running as line.
type navEntry struct {
	Menu, Label, Href, Icon string
	Toggle                  bool
	attrs, inner            string // the entry's attributes and markup, as rendered
}

var (
	navToggleRe  = regexp.MustCompile(`(?s)<(?:a|button)\b[^>]*\bid="navbar-([a-z]+)"[^>]*>(.*?)</(?:a|button)>`)
	navItemRe    = regexp.MustCompile(`(?s)<(?:a|button)\b([^>]*\bclass="[^"]*\b(?:dropdown-item|nav-link)\b[^"]*"[^>]*)>(.*?)</(?:a|button)>`)
	navMenuRe    = regexp.MustCompile(`aria-labelledby="navbar-([a-z]+)"`)
	navHrefRe    = regexp.MustCompile(`\bhref="([^"]*)"`)
	navIconRe    = regexp.MustCompile(`#tabler-([a-z0-9-]+)"`)
	navRunningAs = regexp.MustCompile(`(?s)<div[^>]*\bdata-running-as\b[^>]*>(.*?)</div>`)
	navTagRe     = regexp.MustCompile(`<[^>]+>`)
)

// headerParts cuts a rendered page into its top bar (search, theme, bell,
// New and the user menu) and its main menu.
func headerParts(t *testing.T, body string) (top, main string) {
	t.Helper()
	i := strings.Index(body, `<header class="navbar`)
	j := strings.Index(body, `</header>`)
	k := strings.Index(body, `aria-label="Main">`)
	l := strings.Index(body, `</nav>`)
	if i < 0 || j < i || k < j || l < k {
		t.Fatalf("the page has no top bar and main menu in the expected shape (%d %d %d %d)", i, j, k, l)
	}
	return body[i:j], body[k:l]
}

func navText(inner string) string {
	inner = regexp.MustCompile(`(?s)<span class="d-block small nav-menu-desc">.*?</span>`).ReplaceAllString(inner, "")
	inner = regexp.MustCompile(`(?s)<span class="visually-hidden">.*?</span>`).ReplaceAllString(inner, "")
	return strings.Join(strings.Fields(navTagRe.ReplaceAllString(inner, " ")), " ")
}

// navEntries reads every entry of the main menu, New and the user menu. The
// bell's card is left out: it previews file events and links to the full
// list, which History › File events also opens.
// navSubmenus are menus nested in another (Tabler's .dropend): their entries,
// and the parent's entries after them, belong to the parent.
var navSubmenus = map[string]string{"theme": "user"}

func navEntries(t *testing.T, body string) []navEntry {
	t.Helper()
	top, main := headerParts(t, body)
	var out []navEntry
	for _, part := range []string{main, top} {
		for _, m := range navToggleRe.FindAllStringSubmatch(part, -1) {
			out = append(out, navEntry{Menu: m[1], Label: navText(m[2]), Icon: firstIcon(m[2]), Toggle: true})
		}
		menus := navMenuRe.FindAllStringSubmatchIndex(part, -1)
		for n, m := range menus {
			end := len(part)
			if n+1 < len(menus) {
				end = menus[n+1][0]
			}
			region, menu := part[m[1]:end], part[m[2]:m[3]]
			if part == main {
				// A main-menu dropdown ends at its own </li> (its entries are <a>s):
				// a top-level link after the last one, such as Help, is not in it.
				if i := strings.Index(region, "</li>"); i >= 0 {
					region = region[:i]
				}
			}
			for _, it := range navItemRe.FindAllStringSubmatch(region, -1) {
				if strings.Contains(it[1], `id="navbar-`) {
					continue // the next menu's toggle, read above
				}
				href := ""
				if h := navHrefRe.FindStringSubmatch(it[1]); h != nil {
					href = h[1]
				}
				out = append(out, navEntry{Menu: menu, Label: navText(it[2]), Href: href, Icon: firstIcon(it[2]), attrs: it[1], inner: it[2]})
			}
			if ra := navRunningAs.FindStringSubmatch(region); ra != nil {
				out = append(out, navEntry{Menu: menu, Label: "Running as", Icon: firstIcon(ra[1]), inner: ra[0]})
			}
		}
	}
	// The main menu's own links, outside any dropdown: Overview before the
	// first menu, and Help after the last one.
	first, last := len(main), len(main)
	if ms := navMenuRe.FindAllStringIndex(main, -1); ms != nil {
		first = ms[0][0]
		if i := strings.Index(main[ms[len(ms)-1][1]:], "</li>"); i >= 0 {
			last = ms[len(ms)-1][1] + i
		}
	}
	for i := range out {
		if parent, ok := navSubmenus[out[i].Menu]; ok {
			out[i].Menu = parent
		}
	}
	for _, it := range navItemRe.FindAllStringSubmatch(main[:first]+main[last:], -1) {
		if strings.Contains(it[1], "dropdown-toggle") {
			continue
		}
		h := navHrefRe.FindStringSubmatch(it[1])
		out = append(out, navEntry{Menu: "main", Label: navText(it[2]), Href: h[1], Icon: firstIcon(it[2])})
	}
	return out
}

func firstIcon(inner string) string {
	if m := navIconRe.FindStringSubmatch(inner); m != nil {
		return m[1]
	}
	return ""
}

// newMenuTwins are the deliberate pairs: New repeats the common creates from
// their menu's Tasks (owner, 2026-11-16), so each of its entries has a twin
// with the same page, name and icon. Any other shared icon or page fails.
var newMenuTwins = map[string]string{
	"/hosts#add-host":   "SSH › Tasks › Add a host, repeated under New",
	"/tokens#add-token": "Tokens › Tasks › Add a token, repeated under New",
	"/keys#mint-keys":   "SSH › Tasks › Mint missing keys, repeated under New",
}

// No two entries in the header's menus share an icon or open the same page,
// the user menu included, and every icon they show is in the sprite. The
// only exceptions are New's entries, each a twin of a Tasks entry.
func TestNavEntriesHaveDistinctIconsAndPages(t *testing.T) {
	s, _, _ := newTestServer(t)
	body := signIn(t, s).get("/").Body.String()
	entries := navEntries(t, body)
	if len(entries) < 40 {
		t.Fatalf("read %d menu entries, want at least 40; the reader is not matching the menus", len(entries))
	}
	sprite := string(vendorFile(t, "tabler-icons.svg"))
	byIcon, byHref := map[string][]navEntry{}, map[string][]navEntry{}
	for _, e := range entries {
		if (e.Toggle && e.Menu == "user" && e.Label != "Theme") || e.Label == "Running as" {
			continue // the user menu's toggle and its header show the initials avatar, not an icon
		}
		if e.Icon == "" {
			t.Errorf("%s › %s shows no icon", e.Menu, e.Label)
			continue
		}
		if !strings.Contains(sprite, `id="tabler-`+e.Icon+`"`) {
			t.Errorf("%s › %s uses icon %s, which is not in the sprite; add it to icons.txt", e.Menu, e.Label, e.Icon)
		}
		byIcon[e.Icon] = append(byIcon[e.Icon], e)
		if e.Href != "" && e.Href != "#" {
			byHref[e.Href] = append(byHref[e.Href], e)
		}
	}
	twinOK := func(es []navEntry) bool {
		return len(es) == 2 && (es[0].Menu == "new") != (es[1].Menu == "new") &&
			es[0].Href == es[1].Href && es[0].Icon == es[1].Icon && es[0].Label == es[1].Label &&
			newMenuTwins[es[0].Href] != ""
	}
	twins := 0
	for icon, es := range byIcon {
		if len(es) > 1 && !twinOK(es) {
			t.Errorf("icon %s means more than one thing: %v", icon, es)
		}
	}
	for href, es := range byHref {
		switch {
		case len(es) > 1 && twinOK(es):
			twins++
		case len(es) > 1:
			t.Errorf("%s is in the menus more than once: %v", href, es)
		}
	}
	for _, e := range entries {
		if e.Menu == "new" && !e.Toggle && newMenuTwins[e.Href] == "" {
			t.Errorf("New › %s is not one of the listed twins", e.Label)
		}
	}
	// A stale entry in the list fails too, so it cannot quietly excuse a
	// duplicate added later.
	if twins != len(newMenuTwins) {
		t.Errorf("found %d of the %d listed New twins", twins, len(newMenuTwins))
	}
}

// navHeaders counts a dropdown header by its text in the header's menus.
func navHeaders(t *testing.T, body, text string) int {
	top, main := headerParts(t, body)
	return strings.Count(top+main, `<span class="dropdown-header">`+text+`</span>`)
}

func navHrefs(t *testing.T, body string) map[string]bool {
	hrefs := map[string]bool{}
	for _, e := range navEntries(t, body) {
		hrefs[e.Href] = true
	}
	return hrefs
}

var taskHrefs = []string{
	"/hosts#add-host", "/hosts#knownhosts-init", "/keys#mint-keys",
	"/jobs/review?action=rotate-keys", "/jobs/review?action=revoke-keys",
	"/jobs/review?action=delete-keys", "/tokens#add-token",
}

// A read-only server hides every Tasks header, every task and the New
// button, since it can do none of them. The full server, read the same way,
// shows two Tasks headers (SSH and Tokens), New and every task: the positive
// control that proves the reading can see them.
func TestReadOnlyHidesEveryTaskAndNew(t *testing.T) {
	s, _, _ := newTestServer(t)
	full := signIn(t, s).get("/").Body.String()
	if n := navHeaders(t, full, "Tasks"); n != 2 {
		t.Fatalf("the full server shows %d Tasks headers, want 2; the reading is not matching", n)
	}
	if !strings.Contains(full, `id="navbar-new"`) {
		t.Fatal("the full server has no New button")
	}
	hrefs := navHrefs(t, full)
	for _, h := range taskHrefs {
		if !hrefs[h] {
			t.Errorf("the full server's menus lack the task %s", h)
		}
	}

	p, _ := fixtureHome(t)
	ro, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	body := signIn(t, ro).get("/").Body.String()
	if n := navHeaders(t, body, "Tasks"); n != 0 {
		t.Errorf("a read-only server shows %d Tasks headers", n)
	}
	if strings.Contains(body, `id="navbar-new"`) {
		t.Error("a read-only server shows the New button")
	}
	hrefs = navHrefs(t, body)
	for _, h := range taskHrefs {
		if hrefs[h] {
			t.Errorf("a read-only server's menus offer the task %s", h)
		}
	}
	if !hrefs["/keys"] || len(hrefs) < 15 {
		t.Fatalf("read %d pages from the read-only menus; the reading is not matching", len(hrefs))
	}
}

// The dev sandbox leaves out Rotate and Revoke, which reach real servers,
// and keeps the other tasks.
func TestDevRootHidesRotateAndRevoke(t *testing.T) {
	p, _ := fixtureHome(t)
	p.DevRoot = filepath.Dir(p.SSHDir)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil)})
	if err != nil {
		t.Fatal(err)
	}
	hrefs := navHrefs(t, signIn(t, s).get("/").Body.String())
	for _, h := range []string{"/jobs/review?action=rotate-keys", "/jobs/review?action=revoke-keys"} {
		if hrefs[h] {
			t.Errorf("the dev sandbox's menus offer %s", h)
		}
	}
	for _, h := range []string{"/jobs/review?action=delete-keys", "/keys#mint-keys", "/hosts#knownhosts-init"} {
		if !hrefs[h] {
			t.Errorf("the dev sandbox's menus lack %s, which stays", h)
		}
	}
}
