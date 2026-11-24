package web

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/services/query"
	"github.com/sangehq/authama/src/internal/util/encode"
)

// toolIntegrationsPage shows, for each tool authama can hand its hosts to, which
// hosts and routes that tool gets and which it leaves out, read from the same
// export the download writes, so the page and the file cannot disagree.
type toolIntegrationsPage struct{ s *Server }

// IntegrationsView is the page: the manifest's totals and one card per tool.
type IntegrationsView struct {
	Hosts, Routes int // manifest hosts, and the variant routes rendered beside them
	SSHConfig     string
	Tools         []IntegrationTool
}

// IntegrationTool is one tool: what it imports, the hosts it gets and the
// ones it leaves out with the reason, and how to get the file.
type IntegrationTool struct {
	Name, Label, About string
	// ViaSSH tools run `ssh <alias>`, so every route in ~/.ssh/config works
	// there, proxies included; the others connect on their own.
	ViaSSH   bool
	Included []string
	Skipped  []IntegrationSkip
	Notes    []string
	Page     string // the page with everything about this tool
	Download string
	Command  string
	// Files are what the export writes, previewed on the tool's page; empty
	// for a tool whose own page shows them (OpenSSH, Termius).
	Files []IntegrationFile
}

// IntegrationFile is one file an export writes, as text to preview; Binary
// when it is not text, so the page offers the download instead.
type IntegrationFile struct {
	Name, Body string
	Binary     bool
}

// integrationFile is data as the text a person reads. Windows tools' files are
// UTF-16 with a byte-order mark (PuTTY's .reg, for one), which shown as bytes
// is noise, so it is decoded; anything else that is not UTF-8 is binary.
func integrationFile(name string, data []byte) IntegrationFile {
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return IntegrationFile{Name: name, Body: utf16Text(data[2:], binary.LittleEndian)}
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return IntegrationFile{Name: name, Body: utf16Text(data[2:], binary.BigEndian)}
	}
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(data) {
		return IntegrationFile{Name: name, Binary: true}
	}
	return IntegrationFile{Name: name, Body: string(data)}
}

func utf16Text(data []byte, order binary.ByteOrder) string {
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[2*i:])
	}
	return strings.ReplaceAll(string(utf16.Decode(units)), "\r\n", "\n")
}

// IntegrationHost is a host or route a tool gets, as ssh would reach it.
type IntegrationHost struct {
	Alias, Hostname, User, Route string
	Port                         int
}

// IntegrationDetail is one tool's page.
type IntegrationDetail struct {
	Tool  IntegrationTool
	Hosts []IntegrationHost
}

// IntegrationSkip is a host a tool leaves out, and why.
type IntegrationSkip struct{ Alias, Reason string }

func (tp toolIntegrationsPage) handle(w http.ResponseWriter, r *http.Request) {
	v, err := tp.view()
	if err != nil {
		tp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	tp.s.render(w, http.StatusOK, "integrations", tp.s.newView(r, "integrations", "Tool integrations", v))
}

// handleTool is one tool's page: the hosts it gets with their addresses, the
// ones it leaves out, its import notes and the files its export writes.
func (tp toolIntegrationsPage) handleTool(w http.ResponseWriter, r *http.Request) {
	v, hosts, err := tp.build()
	if err != nil {
		tp.s.renderError(w, r, http.StatusInternalServerError, "Could not read the authama home", err.Error())
		return
	}
	name := r.PathValue("tool")
	for _, t := range v.Tools {
		if t.Name != name {
			continue
		}
		if t.Page != "/integrations/"+t.Name { // OpenSSH and Termius have pages of their own
			http.Redirect(w, r, tp.s.opts.BasePath+t.Page, http.StatusSeeOther)
			return
		}
		d := IntegrationDetail{Tool: t}
		for _, a := range t.Included {
			d.Hosts = append(d.Hosts, hosts[a])
		}
		tp.s.render(w, http.StatusOK, "integration", tp.s.newView(r, "integrations", t.Label, d))
		return
	}
	tp.s.renderError(w, r, http.StatusNotFound, "No such tool", "No tool is called "+strconv.Quote(name)+"; Tool integrations lists them.")
}

func (tp toolIntegrationsPage) view() (IntegrationsView, error) {
	v, _, err := tp.build()
	return v, err
}

// build is every tool, and each alias ssh knows (routes included) as ssh
// reaches it.
func (tp toolIntegrationsPage) build() (IntegrationsView, map[string]IntegrationHost, error) {
	home := tp.s.opts.Engine.Home()
	m, err := manifest.Load(home.Manifest())
	if err != nil {
		return IntegrationsView{}, nil, err
	}
	hosts := map[string]IntegrationHost{}
	at := func(h manifest.Host) {
		hosts[h.Alias] = IntegrationHost{Alias: h.Alias, Hostname: h.Hostname, User: h.User, Port: h.Port, Route: query.RouteOf(h)}
	}
	p := renderer.ExportPathsFor(home.SSHDir)
	v := IntegrationsView{SSHConfig: "~/.ssh/config"}
	var all []string // every alias ssh knows, routes included, in manifest order
	for _, pname := range m.ProfileNames() {
		for _, h := range m.Profiles[pname].Hosts {
			v.Hosts++
			all = append(all, h.Alias)
			at(h)
			for _, x := range h.Variants {
				vh, err := manifest.VariantHostOf(m, pname, h, x)
				if err != nil {
					return IntegrationsView{}, nil, err
				}
				v.Routes++
				all = append(all, vh.Alias)
				at(vh)
			}
		}
	}

	// OpenSSH reads every route: it is the config authama renders.
	v.Tools = append(v.Tools, IntegrationTool{Name: "openssh", Label: "OpenSSH, and anything that runs ssh",
		About:  "The terminal, git, scp, rsync, VS Code Remote and JetBrains Gateway read ~/.ssh/config, so every host and route works there as written.",
		ViaSSH: true, Included: all, Page: "/ssh-config", Download: "/exports/ssh_config",
		Command: brand.Command + " ssh config render"})

	t, err := renderer.RenderTermiusCSV(m, p, renderer.TermiusOptions{})
	if err != nil {
		return IntegrationsView{}, nil, err
	}
	v.Tools = append(v.Tools, tool(IntegrationTool{Name: "termius", Label: "Termius",
		About: "A CSV for Termius's import. It carries no keys: attach each with " + brand.Command + " ssh termius keys.",
		Page:  "/termius", Download: "/exports/termius-hosts.csv",
		Command: brand.Command + " export hosts --format termius"}, all, t.Skipped, nil))

	for _, name := range renderer.ClientFormatNames() {
		f := renderer.ClientFormats[name]
		exp, err := renderer.RenderClientExport(m, p, name, renderer.TermiusOptions{})
		if err != nil {
			return IntegrationsView{}, nil, err
		}
		format := name
		if name == "csv" {
			format = encode.ClientCSV // what --format and a pack call it
		}
		q := url.Values{"part": {"ssh-clients"}, "client": {format}}
		it := tool(IntegrationTool{Name: name, Label: clientLabel(f.Client), About: f.Client,
			ViaSSH: f.ViaSSH, Page: "/integrations/" + name, Download: "/transfer/pack.zip?" + q.Encode(),
			Command: brand.Command + " export hosts --format " + format}, all, exp.Skipped, exp.Notes)
		for _, file := range exp.Files {
			it.Files = append(it.Files, integrationFile(file.Name, file.Data))
		}
		v.Tools = append(v.Tools, it)
	}
	return v, hosts, nil
}

// tool fills t's hosts from the export's skipped list: each entry is
// "<alias>: <reason>", and every alias not in it was written.
func tool(t IntegrationTool, all, skipped, notes []string) IntegrationTool {
	out := map[string]bool{}
	for _, s := range skipped {
		alias, reason, _ := strings.Cut(s, ": ")
		out[alias] = true
		t.Skipped = append(t.Skipped, IntegrationSkip{Alias: alias, Reason: reason})
	}
	sort.Slice(t.Skipped, func(i, j int) bool { return t.Skipped[i].Alias < t.Skipped[j].Alias })
	for _, a := range all {
		if !out[a] {
			t.Included = append(t.Included, a)
		}
	}
	t.Notes = notes
	return t
}

// clientLabel is a client description's name: the part before its first
// " (", so "iTerm2 (Dynamic Profiles)" reads "iTerm2".
func clientLabel(about string) string {
	if strings.HasPrefix(about, "any client") {
		return "Generic CSV"
	}
	if i := strings.Index(about, " ("); i > 0 {
		return about[:i]
	}
	return about
}
