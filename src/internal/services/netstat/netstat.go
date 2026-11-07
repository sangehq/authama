// Package netstat reports per-host network reachability for the `net` verb,
// ported from facade.network_status. Read-only: a fast TCP probe per manifest
// host, with a VPN-aware status message.
package netstat

import (
	"sync"
	"time"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/netcheck"
)

const probeTimeout = 4 * time.Second

// check is the probe; tests swap it for one that touches no network.
var check = netcheck.Check

// probeParallel is how many hosts are probed at once.
const probeParallel = 8

// HostNet is one host's reachability (mirrors facade.HostNet).
type HostNet struct {
	Profile string
	Alias   string
	Status  netcheck.NetStatus
}

// Status returns reachability for every manifest host, filtered by selector: an
// alias, a profile, a "profile/key" reference, or a bare key name.
//
// A bare key name matches in every profile that has one, and says so by
// reporting the profile on each row. Key names are unique per profile, not
// globally, so "imani_github-ed25519" is a legitimate name in several of them -
// the composite form is there for when you mean exactly one.
func Status(m *manifest.Manifest, selector string) ([]HostNet, error) {
	rks, err := m.IterResolved()
	if err != nil {
		return nil, err
	}
	var picked []manifest.ResolvedKey
	for _, rk := range rks {
		h := rk.Host
		ref := manifest.KeyRef{Profile: rk.Profile, KeyName: rk.KeyName}
		if selector != "" && selector != h.Alias && selector != rk.Profile &&
			selector != rk.KeyName && selector != ref.String() {
			continue
		}
		picked = append(picked, rk)
	}
	// Each probe can wait out the timeout, so hosts are probed a few at a time
	// rather than one after another. Rows keep the manifest's order.
	out := make([]HostNet, len(picked))
	sem := make(chan struct{}, probeParallel)
	var wg sync.WaitGroup
	for i, rk := range picked {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, rk manifest.ResolvedKey) {
			defer wg.Done()
			defer func() { <-sem }()
			h := rk.Host
			st := check(h.Hostname, h.Port, h.RequiresVPN, deref(h.VPNName), deref(h.VPNURL), probeTimeout, false)
			out[i] = HostNet{Profile: rk.Profile, Alias: h.Alias, Status: st}
		}(i, rk)
	}
	wg.Wait()
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
