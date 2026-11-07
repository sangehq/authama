package web

import (
	"net/http"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/netstat"
)

// netProbe is `ssh net`'s probe. Tests swap it for one that touches no network.
var netProbe = netstat.Status

// NetView is the /net page: `ssh net [selector]`, run when it is asked for.
// The Hosts, host and Overview pages link here and never probe themselves, so
// no page waits on the network to load.
type NetView struct {
	// Selector is the host alias or profile the page was asked for; "" is every host.
	Selector string
	// IsProfile is true when Selector names a profile rather than a host.
	IsProfile bool
	Command   string
	Rows      []NetRow
	// GatedDown counts the unreachable hosts that need a VPN, the rows that make
	// `ssh net` exit non-zero.
	GatedDown int
	Online    int
}

// NetRow is one host's reachability, worded as `ssh net` words it.
type NetRow struct {
	Profile, Alias, Message string
	Online, RequiresVPN     bool
	VPNName                 string
}

// netPage serves /net, kept off pages, which the layering test holds to a
// ceiling of methods.
type netPage struct{ s *Server }

// handleNet probes on request. It takes a host alias or a profile name from the
// query, checked against the manifest, so the page probes only hosts the
// manifest names: no free hostname or port reaches it from a link.
func (n netPage) handleNet(w http.ResponseWriter, r *http.Request) {
	h := pages(n)
	m, err := manifest.Load(h.s.opts.Engine.Home().Manifest())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	selector, isProfile := r.URL.Query().Get("host"), false
	if selector == "" {
		selector, isProfile = r.URL.Query().Get("profile"), r.URL.Query().Get("profile") != ""
	}
	if selector != "" && !netSelectable(m, selector) {
		h.fail(w, r, ErrNotFound)
		return
	}
	rows, err := netProbe(m, selector)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	v := NetView{Selector: selector, IsProfile: isProfile, Command: brand.Command + " ssh net"}
	if selector != "" {
		v.Command += " " + selector
	}
	for _, hn := range rows {
		row := NetRow{Profile: hn.Profile, Alias: hn.Alias, Message: hn.Status.Message(),
			Online: hn.Status.Reachable, RequiresVPN: hn.Status.RequiresVPN, VPNName: hn.Status.VPNName}
		if row.Online {
			v.Online++
		} else if row.RequiresVPN {
			v.GatedDown++
		}
		v.Rows = append(v.Rows, row)
	}
	title := "Connections"
	if selector != "" {
		title = "Connection: " + selector
	}
	h.s.render(w, http.StatusOK, "net", h.s.newView(r, "hosts", title, v))
}

// netSelectable is true for a host alias or a profile name that `ssh net`
// would probe: one in the manifest's resolved hosts.
func netSelectable(m *manifest.Manifest, name string) bool {
	rks, err := m.IterResolved()
	if err != nil {
		return false
	}
	for _, rk := range rks {
		if rk.Profile == name || rk.Host.Alias == name {
			return true
		}
	}
	return false
}
