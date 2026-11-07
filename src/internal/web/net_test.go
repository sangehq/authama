package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/netstat"
	"github.com/simtabi/authama/src/internal/util/netcheck"
)

// fakeNet replaces ssh net's probe with one that touches no network, and
// records the selectors it was asked for.
func fakeNet(t *testing.T, rows []netstat.HostNet) *[]string {
	t.Helper()
	var asked []string
	prev := netProbe
	netProbe = func(_ *manifest.Manifest, selector string) ([]netstat.HostNet, error) {
		asked = append(asked, selector)
		var out []netstat.HostNet
		for _, r := range rows {
			if selector == "" || selector == r.Alias || selector == r.Profile {
				out = append(out, r)
			}
		}
		return out, nil
	}
	t.Cleanup(func() { netProbe = prev })
	return &asked
}

var netRows = []netstat.HostNet{
	{Profile: "work", Alias: "gh", Status: netcheck.NetStatus{Host: "github.com", Port: 22, Reachable: true}},
	{Profile: "work", Alias: "db", Status: netcheck.NetStatus{Host: "db.internal", Port: 22, RequiresVPN: true, VPNName: "office"}},
}

// Probing waits on the network, so no page probes as it loads: the pages
// that offer a check link to /net, and only /net probes.
func TestNoPageProbesAsItLoads(t *testing.T) {
	asked := fakeNet(t, netRows)
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for path, link := range map[string]string{"/": `/net"`, "/hosts": `/net"`, "/hosts/gh": `/net?host=gh"`} {
		body := c.get(path).Body.String()
		if !strings.Contains(body, link) || !strings.Contains(body, "Check connection") {
			t.Errorf("%s offers no connection check (%s)", path, link)
		}
	}
	if len(*asked) != 0 {
		t.Fatalf("a page probed as it loaded: %q", *asked)
	}
}

// /net is `ssh net`: each host's status in the command's own words, and a
// warning for the VPN-gated host that makes the command exit non-zero.
func TestTheNetPageIsTheCommand(t *testing.T) {
	asked := fakeNet(t, netRows)
	s, _, _ := newTestServer(t)
	c := signIn(t, s)

	rec := c.get("/net")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("/net: %d\n%s", rec.Code, body)
	}
	for _, want := range []string{
		"authama ssh net", "1 of 2 reachable", "github.com:22 reachable",
		"db.internal:22 unreachable - this host requires a VPN (office)", "exits non-zero",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/net does not say %q", want)
		}
	}

	body = c.get("/net?host=gh").Body.String()
	if !strings.Contains(body, "authama ssh net gh") || strings.Contains(body, "db.internal") || strings.Contains(body, "exits non-zero") {
		t.Errorf("/net?host=gh is not `ssh net gh`:\n%s", body)
	}
	if body := c.get("/net?profile=work").Body.String(); !strings.Contains(body, "?profile=work") {
		t.Errorf("Check again should keep the profile:\n%s", body)
	}
	if got := strings.Join(*asked, ","); got != ",gh,work" {
		t.Errorf("probed %q, want every host, then gh, then work", got)
	}
}

// The page probes manifest hosts only: a name the manifest does not have is a
// 404 before anything is probed, so no free hostname or port arrives by link.
func TestNetTakesOnlyManifestNames(t *testing.T) {
	asked := fakeNet(t, netRows)
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for _, q := range []string{"host=nope", "host=-oProxyCommand%3Dx", "profile=nope", "host=github.com"} {
		if rec := c.get("/net?" + q); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d", q, rec.Code)
		}
	}
	if len(*asked) != 0 {
		t.Errorf("an unknown name was probed: %v", *asked)
	}
}

// Reading reachability writes nothing, so a read-only server offers it too.
func TestAReadOnlyServerChecksConnections(t *testing.T) {
	fakeNet(t, netRows)
	p, _ := fixtureHome(t)
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken, AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	if !strings.Contains(c.get("/hosts").Body.String(), `/net"`) {
		t.Error("the read-only Hosts page offers no connection check")
	}
	if rec := c.get("/net"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "1 of 2 reachable") {
		t.Errorf("/net on a read-only server: %d", rec.Code)
	}
}
