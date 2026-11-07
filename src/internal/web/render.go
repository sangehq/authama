package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/expiry"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/owner"
	"github.com/simtabi/authama/src/internal/util/highlight"
)

// assetSet is the embedded assets with a content hash each, for
// fingerprinted, immutable URLs.
type assetSet struct {
	files  map[string][]byte // path below assets/
	hashes map[string]string // first 12 hex chars of sha256
	sprite template.HTML
}

func loadAssets() (*assetSet, error) {
	a := &assetSet{files: map[string][]byte{}, hashes: map[string]string{}}
	err := fs.WalkDir(assetFS, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := assetFS.ReadFile(p)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(p, "assets/")
		sum := sha256.Sum256(body)
		a.files[name] = body
		a.hashes[name] = hex.EncodeToString(sum[:])[:12]
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The JavaScript is ES modules that import each other by relative path,
	// so app.js and everything under js/ share one fingerprint, taken over
	// all of them: a relative import lands under the same /assets/<hash>/ and
	// still finds its file, and any change to any module moves them all.
	group := sha256.New()
	var mods []string
	for name := range a.files {
		if isModule(name) {
			mods = append(mods, name)
		}
	}
	sort.Strings(mods)
	for _, name := range mods {
		group.Write([]byte(name))
		group.Write(a.files[name])
	}
	modHash := hex.EncodeToString(group.Sum(nil))[:12]
	for _, name := range mods {
		a.hashes[name] = modHash
	}
	sprite, ok := a.files["vendor/tabler-icons.svg"]
	if !ok {
		return nil, fmt.Errorf("web: the icon sprite is not embedded")
	}
	// The sprite is vendored, hash-checked by a test, and contains no script:
	// it is inlined so <use href="#..."> stays same-document.
	a.sprite = template.HTML(sprite) //nolint:gosec // trusted vendored file, see above
	return a, nil
}

// isModule reports whether an asset is one of the UI's ES modules.
func isModule(name string) bool { return name == "app.js" || strings.HasPrefix(name, "js/") }

// url returns the fingerprinted URL of an asset. Asking for one that does not
// exist fails the template, which the page tests turn into a failure rather
// than a broken page.
func (a *assetSet) url(name string) (string, error) {
	h, ok := a.hashes[name]
	if !ok {
		return "", fmt.Errorf("no embedded asset %q", name)
	}
	return "/assets/" + h + "/" + name, nil
}

// pageSet is every page template, each parsed once over the shared layout.
type pageSet struct {
	pages map[string]*template.Template
}

func pageNames() ([]string, error) {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "layout.html" || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".html"))
	}
	sort.Strings(out)
	return out, nil
}

func funcs(a *assetSet, basePath string) template.FuncMap {
	return template.FuncMap{
		"asset": func(name string) (string, error) {
			u, err := a.url(name)
			return basePath + u, err
		},
		// basePath is the path the UI is served under ("" at the root): every
		// literal path in a template starts {{basePath}}/, which a test checks.
		"basePath": func() string { return basePath },
		// u puts the base path in front of a path built in Go ("/hosts/new")
		// where a template writes it out.
		"u":      func(p string) string { return prefixed(basePath, p) },
		"sprite": func() template.HTML { return a.sprite },
		"icon":   iconHTML,
		// The product name and the command line for a recorded verb, from
		// the one place they are decided.
		"brandName":    func() string { return brand.Name },
		"brandCommand": func() string { return brand.Command },
		"cmdline":      brand.Line,
		"when":         func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
		"stamp": func(s string) string {
			if t, err := time.Parse(history.TimeFormat, s); err == nil {
				return t.UTC().Format("2006-01-02 15:04:05 UTC")
			}
			return s
		},
		"orDash": func(s string) string {
			if s == "" {
				return "–"
			}
			return s
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"join":        strings.Join,
		"list":        func(xs ...string) []string { return xs },
		"hypervisors": manifest.KnownHypervisors,
		"short":       func(s string) string { return shorten(s, 8) },
		"size":        humanSize,
		"resultClass": func(r string) string {
			switch r {
			case history.ResultOK:
				return "badge-ok"
			case history.ResultDryRun:
				return "badge-info"
			case history.ResultAborted:
				return "badge-muted"
			case "":
				return "badge-warn"
			}
			return "badge-bad"
		},
		"statusClass": func(s string) string {
			switch s {
			case "deployed", "ok", "running":
				return "badge-ok"
			case "not-deployed", "needs-redeploy", "due_soon", "stopped", "suspended", "paused", "declared":
				return "badge-warn"
			case "no-key", "overdue", "gone":
				return "badge-bad"
			}
			return "badge-muted"
		},
		"expiryDays": func(s expiry.Status) string {
			if s.DaysRemaining == nil {
				return "–"
			}
			if *s.DaysRemaining < 0 {
				return fmt.Sprintf("%d days ago", -*s.DaysRemaining)
			}
			return fmt.Sprintf("in %d days", *s.DaysRemaining)
		},
		"diffClass": func(op byte) string {
			switch op {
			case '+':
				return "diff-add"
			case '-':
				return "diff-del"
			case '@':
				return "diff-hunk"
			}
			return "diff-ctx"
		},
		"fileDiff": newDiffView,
		"commandCard": func(command string, runnable bool, form string, destroy bool) commandCard {
			return commandCard{Command: command, Runnable: runnable, Form: form, Destroy: destroy}
		},
		"diffOf": func(path string, lines []history.DiffLine) diffView {
			return newDiffView(path, "", lines, false, "")
		},
		"hl":    func(lang, line string) template.HTML { return highlightHTML(highlight.Lang(lang), line) },
		"hlFor": func(path, line string) template.HTML { return highlightHTML(highlight.For(path), line) },
		"lines": func(s string) []string { return strings.Split(strings.TrimSuffix(s, "\n"), "\n") },
		"diffMark": func(op byte) string {
			if op == '@' {
				return "⋯"
			}
			return string(op)
		},
		"subjects": func(subs []history.Subject) string {
			var parts []string
			for _, s := range subs {
				parts = append(parts, s.Kind+" "+s.Name)
			}
			return strings.Join(parts, ", ")
		},
		// subjectsShort is the first few subjects and how many more there
		// are, so one operation over many keys does not make a tall row.
		"subjectsShort": func(subs []history.Subject) string {
			const shown = 3
			var parts []string
			for i, s := range subs {
				if i == shown {
					return strings.Join(parts, ", ") + fmt.Sprintf(" and %d more", len(subs)-shown)
				}
				parts = append(parts, s.Kind+" "+s.Name)
			}
			return strings.Join(parts, ", ")
		},
		"base": filepath.Base,
		// pct is part of whole as a percentage, rounded to the 5% steps
		// app.css has width classes for (no inline style passes the CSP).
		"pct": func(part, whole int) int {
			if whole <= 0 || part <= 0 {
				return 0
			}
			p := (part*100 + whole/2) / whole
			return min(100, (p+2)/5*5)
		},
		"sub": func(a, b int) int { return a - b },
		"eventDot": func(kind string) string {
			switch kind {
			case events.KindAdded:
				return "status-dot-animated status-green"
			case events.KindRemoved:
				return "status-red"
			case events.KindMode:
				return "status-yellow"
			case events.KindChanged:
				return "status-azure"
			}
			return "status-secondary"
		},
		"eventBadge": func(kind string) string {
			switch kind {
			case events.KindAdded:
				return "badge-ok"
			case events.KindRemoved:
				return "badge-bad"
			case events.KindMode:
				return "badge-warn"
			case events.KindChanged:
				return "badge-info"
			}
			return "badge-muted"
		},
		"confirm": func(id, form, title, text, label string) confirmModal {
			return confirmModal{ID: id, Form: form, Title: title, Text: text, Label: label}
		},
		"withPurge":       func(m confirmModal) confirmModal { m.Purge = true; return m },
		"help":            func(title, text string) helpTip { return helpTip{Title: title, Text: text} },
		"copy":            func(value, what string) copyTip { return copyTip{Value: value, What: what} },
		"paletteItems":    paletteItems,
		"paletteSections": paletteSections,
		"shortcutKeys":    shortcutKeys,
		"stat": func(label string, count int, href, icon, colour string) stat {
			return stat{Label: label, Count: count, Href: href, Icon: icon, Colour: colour}
		},
		"empty": func(icon, title, text string) emptyState {
			return emptyState{Icon: icon, Title: title, Text: text}
		},
		// footer is a panel form's footer: the command it matches, the submit
		// button's label, its style and its icon ("" for none).
		"footer": func(command, label, style, icon string) panelFooter {
			return panelFooter{Command: command, Label: label, Style: style, Icon: icon}
		},
		// crumbs is a breadcrumb trail from label, path pairs; an empty path
		// is the current page.
		"crumbs": func(pairs ...string) []crumb {
			out := make([]crumb, 0, len(pairs)/2)
			for i := 0; i+1 < len(pairs); i += 2 {
				out = append(out, crumb{Label: pairs[i], Href: pairs[i+1]})
			}
			return out
		},
		"header": func(pretitle, title, subtitle string, actions ...headerAction) pageHeader {
			h := pageHeader{Pretitle: pretitle, Title: title, Subtitle: subtitle}
			for _, a := range actions {
				if a.Label != "" { // an action `when` left out
					h.Actions = append(h.Actions, a)
				}
			}
			return h
		},
		// only keeps an action if cond holds, so a header lists the
		// actions a read-only or dev-mode server does not offer in one place.
		"only": func(cond bool, a headerAction) headerAction {
			if !cond {
				return headerAction{}
			}
			return a
		},
		"action": func(label, href, icon string) headerAction {
			return headerAction{Label: label, Href: href, Icon: icon}
		},
		// inline is an action whose page opens in the side panel.
		"inline": func(label, href, icon string) headerAction {
			return headerAction{Label: label, Href: href, Icon: icon, Inline: true}
		},
		// download is an action that downloads a file; a CSV opens the CSV dialog first.
		"download": func(label, href string) headerAction {
			return headerAction{Label: label, Href: href, Icon: "download", Download: true,
				CSV: strings.HasSuffix(strings.SplitN(href, "?", 2)[0], ".csv")}
		},
		// tip is ` data-tip="…"` from the catalogue in tips.go, for any element.
		"tip": tipAttr,
		// fixFor is the web UI's way of doing an overview row's fix, or nil.
		"fixFor": fixFor,
		// withTip gives a header action its explainer:
		// (panel "Add a host" "add-host" "plus" | withTip "host-add").
		"withTip": func(id string, a headerAction) (headerAction, error) {
			t, err := tipText(id)
			a.Tip = t
			return a, err
		},
		// modal is an action that opens the dialog with that id.
		"modal": func(label, id, icon string) headerAction {
			return headerAction{Label: label, Href: "#" + id, Icon: icon, Modal: id}
		},
		// panel is an action that slides in the offcanvas panel with that id.
		"panel": func(label, id, icon string) headerAction {
			return headerAction{Label: label, Href: "#" + id, Icon: icon, Panel: id}
		},
		"navGroup": func(_ view, key, icon, label string, items ...navItem) navGroup {
			g := navGroup{Key: key, Icon: icon, Label: label, Tip: groupTips[key], Items: items}
			for _, it := range items {
				g.Active = g.Active || it.Active
			}
			return g
		},
		"navItem": func(v view, key, href, icon, label string) navItem {
			return navItem{Active: v.Nav == key, Href: href, Icon: icon, Label: label, Tip: pageTips[key]}
		},
	}
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// iconHTML renders a Tabler icon from the inline sprite. Icons are
// decorative here: every one sits beside text that says the same thing.
func iconHTML(name string) template.HTML {
	if !validIcon(name) {
		return ""
	}
	return template.HTML(`<svg class="icon" aria-hidden="true" focusable="false"><use href="#tabler-` + name + `"></use></svg>`) //nolint:gosec // name is [a-z0-9-] only
}

func validIcon(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func parsePages(a *assetSet, basePath string) (*pageSet, error) {
	base, err := template.New("layout.html").Funcs(funcs(a, basePath)).ParseFS(templateFS, "templates/layout.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}
	names, err := pageNames()
	if err != nil {
		return nil, err
	}
	ps := &pageSet{pages: map[string]*template.Template{}}
	for _, n := range names {
		t, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if _, err := t.ParseFS(templateFS, path.Join("templates", n+".html")); err != nil {
			return nil, err
		}
		ps.pages[n] = t
	}
	return ps, nil
}

// pageHeader is a page's header: the section, the title, and a one-line
// summary under it.
type pageHeader struct {
	Pretitle, Title, Subtitle string
	Actions                   []headerAction
}

// headerAction is a link at the right of a page header, to a form further
// down the page; the first is the primary one.
type headerAction struct {
	Label, Href, Icon, Panel, Modal string
	Download, CSV                   bool
	Inline                          bool // opens in the side panel (data-inline)
	// Tip is the explainer the action shows on hover and focus (see tips.go).
	Tip string
}

// emptyState is a list with nothing in it yet.
type emptyState struct{ Icon, Title, Text string }

// Subtitle is Text with each `backticked` span set as code, so an empty state
// can name the command that fills it. Everything is escaped first; only the
// <code> tags this adds are markup.
func (e emptyState) Subtitle() template.HTML {
	parts := strings.Split(e.Text, "`")
	if len(parts)%2 == 0 { // an unclosed backtick is text, not code
		return template.HTML(template.HTMLEscapeString(e.Text)) //nolint:gosec // escaped
	}
	var b strings.Builder
	for i, part := range parts {
		esc := template.HTMLEscapeString(part)
		if i%2 == 1 {
			b.WriteString("<code>" + esc + "</code>")
		} else {
			b.WriteString(esc)
		}
	}
	return template.HTML(b.String()) //nolint:gosec // every part escaped above
}

// confirmModal asks before a destructive action is previewed.
type confirmModal struct {
	ID, Form, Title, Text, Label string
	Purge                        bool // offer "and its key files", for a profile delete
}

// stat is one count on the overview.
type stat struct {
	Label              string
	Count              int
	Href, Icon, Colour string
}

// navGroup is a menu in the top bar: its pages, in a dropdown.
type navGroup struct {
	Key, Icon, Label string
	Tip              string // what the menu holds (groupTips)
	Active           bool
	Items            []navItem
}

type navItem struct {
	Active            bool
	Href, Icon, Label string
	Tip               string // the page's one-line description (pageTips)
}

// view is what every page template receives.
type view struct {
	Title    string
	Nav      string
	Authed   bool
	CSRF     string
	Version  string
	ReadOnly bool
	Home     string
	SSHDir   string
	DevRoot  string
	Data     any
	// Bell is the newest file events, for the header's notifications menu.
	Bell []events.Event
	// BellNew is how many of them are from the last day: the bell's dot
	// means "something changed recently", not "there was ever an event".
	BellNew int
	// User and Host are the account and machine the server runs as, and
	// Initials the avatar's letters; Year is for the footer.
	User, Host, Initials string
	Year                 int
	// Panel is a request from the side panel: the page's content is rendered
	// alone, with no layout around it.
	Panel bool
	// PINNeeded is a session that must give the PIN to confirm a change: the
	// confirmations show a PIN field when it is set.
	PINNeeded bool
	// SealProblem is the owner seal's finding, shown on the overview when the
	// state was changed outside authama.
	SealProblem string
	// HideExport leaves Export out of the menu (web.menu_export false).
	HideExport bool
	// Build is the running build, for About's current-build status:
	// its version, when its binary was built, and when the server started.
	Build buildInfo
	// Instance is the server's instance id, so a page can tell when a new
	// server (a new build) answers on its address.
	Instance string
	// CanRestart is a server that can restart itself from the stale banner.
	CanRestart bool
	// Rebuilt is the server's own executable when it changed after the
	// server started: the page is the old build, and the layout says so.
	Rebuilt string
}

func (s *Server) newView(r *http.Request, nav, title string, data any) view {
	p := s.opts.Engine.Home()
	v := view{Title: title, Nav: nav, Version: s.opts.Version, ReadOnly: s.opts.ReadOnly, Data: data,
		Home: p.ConfigDir, SSHDir: p.SSHDir, DevRoot: p.DevRoot,
		User: s.who.user, Host: s.who.host, Initials: s.who.initials(), Year: s.now().Year(),
		Panel: r.Header.Get(panelHeader) == "1"}
	if sess := sessionFrom(r.Context()); sess != nil {
		v.Authed = true
		v.CSRF = sess.csrf
		v.PINNeeded = ownerGate{s}.needsStepUp(sess)
		v.Rebuilt = s.binary.replaced()
		v.HideExport = s.opts.HideExport
		v.Build = s.binary.info(s.opts.Version)
		v.CanRestart = s.opts.Restart != nil
		v.Instance = s.opts.Instance
		if nav == "overview" || nav == "doctor" {
			if st := owner.Verify(p); !st.Clean() {
				v.SealProblem = st.Describe()
			}
		}
		if s.opts.EventRoots != nil {
			v.Bell, _ = events.List(p, events.Filter{Limit: 6})
			for _, e := range v.Bell {
				if s.now().Sub(e.Time) < 24*time.Hour {
					v.BellNew++
				}
			}
		}
	}
	return v
}

// render executes a page into a buffer first, so a template error becomes a
// clean 500 instead of half a page.
func (s *Server) render(w http.ResponseWriter, status int, page string, v view) {
	t, ok := s.pages.pages[page]
	if !ok {
		http.Error(w, "no such page template: "+page, http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	frame := "layout"
	if v.Panel {
		frame = "panel"
	}
	if err := t.ExecuteTemplate(&buf, frame, v); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// The same URL answers with a page or a panel's content by this header.
	w.Header().Add("Vary", panelHeader)
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// commandCard is the "The same as running" card: the command, a copy
// button, and a Run button that submits the page's own confirmation form.
type commandCard struct {
	Command  string
	Runnable bool
	Form     string
	Destroy  bool
}

// panelHeader marks a request from the side panel (see js/components/side-panel.js).
const panelHeader = "X-Authama-Panel"

type errorData struct {
	Status         int
	Heading        string
	Message        string
	Link, LinkText string
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, heading, message string) {
	s.render(w, status, "error", s.newView(r, "", heading, errorData{Status: status, Heading: heading, Message: message}))
}

func (s *Server) renderUnauthorized(w http.ResponseWriter, r *http.Request) {
	s.renderError(w, r, http.StatusUnauthorized, "Sign-in link required",
		"This browser is not signed in to this server. Open the address `authama serve` printed when it started: "+
			"it carries the token that signs a browser in. A server that was restarted prints a new one, "+
			"and the old address and sessions stop working.")
}

// serveAsset answers /assets/{hash}/{path...}. A stale hash is a 404, not the
// current file: an immutable URL must never change content.
func (s *Server) serveAsset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	body, ok := s.assets.files[name]
	if !ok || r.PathValue("hash") != s.assets.hashes[name] {
		http.NotFound(w, r)
		return
	}
	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(body)
}

// paletteItem is one entry of the command palette. Keys is its keyboard
// shortcut, a key sequence such as "g h" (press g, then h), or empty.
type paletteItem struct {
	ID, Label, Href, Icon, Group, Keywords, Keys string
	Tip                                          string // what choosing it does (paletteTip), set by paletteSections
}

// paletteSection is a titled run of palette entries: the pages, the forms
// that open in a panel, or the downloads.
type paletteSection struct {
	Title string
	Items []paletteItem
}

// paletteSections is paletteItems in the order the palette shows them, under
// a heading each: Pages, Actions, Downloads.
func paletteSections(readOnly bool) []paletteSection {
	secs := []paletteSection{{Title: "Pages"}, {Title: "Actions"}, {Title: "Downloads"}}
	for _, it := range paletteItems(readOnly) {
		switch {
		case it.Group == "Action":
			secs[1].Items = append(secs[1].Items, it)
		case strings.HasPrefix(it.Href, "/exports/"):
			secs[2].Items = append(secs[2].Items, it)
		default:
			secs[0].Items = append(secs[0].Items, it)
		}
	}
	for i := range secs {
		for j := range secs[i].Items {
			secs[i].Items[j].Tip = paletteTip(secs[i].Items[j])
		}
	}
	out := secs[:0]
	for _, sec := range secs {
		if len(sec.Items) > 0 {
			out = append(out, sec)
		}
	}
	return out
}

// paletteTip is what choosing a palette entry does: a page's description,
// the explainer of the form it opens, or the download's.
func paletteTip(it paletteItem) string {
	if t, ok := pageTips[it.ID]; ok {
		return t
	}
	if strings.HasPrefix(it.Href, "/exports/") {
		return tips["download"]
	}
	if t, ok := tips[strings.TrimPrefix(it.ID, "add-")+"-add"]; ok {
		return t
	}
	return ""
}

// shortcutKeys splits a shortcut such as "g h" into the keys to press, for
// one <kbd> each.
func shortcutKeys(keys string) []string { return strings.Fields(keys) }

// paletteItems is what the command palette lists: every page, the forms that
// open in a panel (not in read-only mode), and the downloads.
func paletteItems(readOnly bool) []paletteItem {
	items := []paletteItem{
		{"overview", "Overview", "/", "layout-dashboard", "Page", "home dashboard state", "g o", ""},
		{"profiles", "Profiles", "/profiles", "users", "SSH", "profile category key scope", "g p", ""},
		{"naming", "Naming", "/naming", "license", "SSH", "naming rules alias template key name code env role check", "g n", ""},
		{"hosts", "Hosts", "/hosts", "server", "SSH", "host alias hostname", "g h", ""},
		{"keys", "Keys", "/keys", "key", "SSH", "key fingerprint expiry reserve", "g k", ""},
		{"vms", "VMs", "/vms", "device-desktop", "SSH", "vm hypervisor devbox sync", "g v", ""},
		{"sshconfig", "SSH config", "/ssh-config", "file-code", "Config & backups", "config render drift check", "g c", ""},
		{"snapshots", "Snapshots", "/snapshots", "archive", "Config & backups", "snapshot bundle backup restore", "g s", ""},
		{"tokens", "Auth tokens", "/tokens", "cloud-lock", "Security", "token npm composer docker git netrc pypi cargo gem gradle maven yarn helm gh", "g t", ""},
		{"audit", "Audit", "/audit", "shield-check", "Security", "audit key age", "g a", ""},
		{"security", "Owner & PIN", "/security", "lock", "Security", "owner pin seal tamper binding fingerprint", "g w", ""},
		{"history", "History", "/history", "history", "Activity", "history operations changes log", "g y", ""},
		{"events", "Events", "/events", "activity", "Activity", "events file changes tracking", "g e", ""},
		{"export", "Export", "/export", "file-export", "Export", "export download csv json yaml php javascript markdown html filter columns", "g x", ""},
		{"doctor", "Doctor", "/doctor", "stethoscope", "System", "doctor checks fix", "g d", ""},
		{"validate", "Validate", "/validate", "circle-check", "System", "validate check manifest settings keys keypair config schema", "g l", ""},
		{"repair", "Repair", "/repair", "refresh", "System", "repair fix corrupt permissions factory reset", "g r", ""},
		{"settings", "Settings", "/settings", "settings", "System", "settings theme appearance env", "g ,", ""},
		{"about", "About", "/about", "info-circle", "System", "about version licences", "g i", ""},
	}
	if !readOnly {
		items = append(items,
			paletteItem{"add-profile", "Add a profile", "/profiles#add-profile", "plus", "Action", "new create profile", "n p", ""},
			paletteItem{"add-host", "Add a host", "/hosts#add-host", "plus", "Action", "new create host", "n h", ""},
			paletteItem{"add-token", "Add or replace a token", "/tokens#add-token", "plus", "Action", "new set rotate token secret", "n t", ""},
		)
	}
	return append(items,
		paletteItem{"export-config", "Download ssh_config, every host inline", "/exports/ssh_config", "download", "Export", "export flat ssh_config", "", ""},
		paletteItem{"export-termius", "Download the Termius CSV", "/exports/termius-hosts.csv", "download", "Export", "export termius csv", "", ""},
		paletteItem{"export-hosts", "Download the hosts as CSV", "/exports/hosts.csv", "download", "Export", "export csv hosts spreadsheet", "", ""},
		paletteItem{"export-keys", "Download the keys as CSV", "/exports/keys.csv", "download", "Export", "export csv keys spreadsheet", "", ""},
		paletteItem{"export-tokens", "Download the installed tokens as CSV", "/exports/tokens.csv", "download", "Export", "export csv tokens spreadsheet", "", ""},
		paletteItem{"export-history", "Download the history as CSV", "/exports/history.csv", "download", "Export", "export csv history operations", "", ""},
		paletteItem{"export-events", "Download the file events as CSV", "/exports/events.csv", "download", "Export", "export csv events audit", "", ""},
		paletteItem{"export-profiles", "Download the profiles as CSV", "/exports/profiles.csv", "download", "Export", "export csv profiles spreadsheet", "", ""},
		paletteItem{"export-snapshots", "Download the snapshots as CSV", "/exports/snapshots.csv", "download", "Export", "export csv snapshots backups", "", ""},
		paletteItem{"export-deployments", "Download the key deployments as CSV", "/exports/deployments.csv", "download", "Export", "export csv deployments audit keys", "", ""},
	)
}

// helpTip is a "what is this" popover: a title and a sentence, both text.
type helpTip struct{ Title, Text string }

// copyTip is a copy button for a public value, and what to call it.
type copyTip struct{ Value, What string }

// crumb is one step of a detail page's breadcrumb.
type crumb struct {
	Label, Href string
}

// panelFooter is a panel form's footer.
type panelFooter struct {
	Command, Label, Style, Icon string
}
