package web

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
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
	Page     string // an authama page with more on this tool, if any
	Download string
	Command  string
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

func (tp toolIntegrationsPage) view() (IntegrationsView, error) {
	home := tp.s.opts.Engine.Home()
	m, err := manifest.Load(home.Manifest())
	if err != nil {
		return IntegrationsView{}, err
	}
	p := renderer.ExportPathsFor(home.SSHDir)
	v := IntegrationsView{SSHConfig: "~/.ssh/config"}
	var all []string // every alias ssh knows, routes included, in manifest order
	for _, pname := range m.ProfileNames() {
		for _, h := range m.Profiles[pname].Hosts {
			v.Hosts++
			all = append(all, h.Alias)
			for _, x := range h.Variants {
				v.Routes++
				all = append(all, manifest.VariantAlias(x.Name, h.Alias))
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
		return IntegrationsView{}, err
	}
	v.Tools = append(v.Tools, tool(IntegrationTool{Name: "termius", Label: "Termius",
		About: "A CSV for Termius's import. It carries no keys: attach each with " + brand.Command + " ssh termius keys.",
		Page:  "/termius", Download: "/exports/termius-hosts.csv",
		Command: brand.Command + " export hosts --format termius"}, all, t.Skipped, nil))

	for _, name := range renderer.ClientFormatNames() {
		f := renderer.ClientFormats[name]
		exp, err := renderer.RenderClientExport(m, p, name, renderer.TermiusOptions{})
		if err != nil {
			return IntegrationsView{}, err
		}
		format := name
		if name == "csv" {
			format = encode.ClientCSV // what --format and a pack call it
		}
		q := url.Values{"part": {"ssh-clients"}, "client": {format}}
		v.Tools = append(v.Tools, tool(IntegrationTool{Name: name, Label: clientLabel(f.Client), About: f.Client,
			ViaSSH: f.ViaSSH, Download: "/transfer/pack.zip?" + q.Encode(),
			Command: brand.Command + " export hosts --format " + format}, all, exp.Skipped, exp.Notes))
	}
	return v, nil
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
