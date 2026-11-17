package rotator

import (
	"fmt"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/providers"
	"github.com/sangehq/authama/src/internal/util/netcheck"
)

// HostCheck is what a dry run found for one host a key is deployed to.
type HostCheck struct {
	Alias, Provider string
	// Manual is a target that takes the new key by hand: the rotation stops
	// there and asks you to paste it.
	Manual bool
	// OK is false when the rotation would fail here; Message says why.
	OK      bool
	Message string
	// Kind names what was found, for a summary: KindLogsIn, KindAPI,
	// KindManual, KindUnreachable, KindNeverDeployed or KindRemoved.
	Kind string
	// Via is how the new key would reach the host: ssh, a provider's API, or
	// a provider that takes it by hand.
	Via string
}

// What a dry run can find at a host, and why a key cannot rotate.
const (
	KindLogsIn        = "logs-in"
	KindAPI           = "api"
	KindManual        = "manual"
	KindUnreachable   = "unreachable"
	KindNeverDeployed = "never-deployed"
	KindRemoved       = "removed"
	KindNoKey         = "no-key"
	KindFailed        = "failed"
	// KindOldLive is a key whose previous key in old/ still logs in
	// somewhere, kept by the policy: the rotation would stop rather than
	// drop it. KindEvict is the same key under the evict policy: it would be
	// taken off those hosts first.
	KindOldLive = "old-live"
	KindEvict   = "evict-old"
)

// CheckReport is a dry run of one key's rotation.
type CheckReport struct {
	Key   string
	Hosts []HostCheck
	// Problem is set when the key cannot be rotated at all (no key file);
	// ProblemKind names it.
	Problem, ProblemKind string
	// Evict names the hosts the previous key in old/ would be taken off
	// before it is dropped (the evict policy, Rotator.EvictOld).
	Evict []string
}

// OK reports whether the rotation would go through without a failure.
func (c CheckReport) OK() bool {
	if c.Problem != "" {
		return false
	}
	for _, h := range c.Hosts {
		if !h.OK {
			return false
		}
	}
	return true
}

// Check is a dry run of Rotate: it changes nothing, and tells for each host
// the key is used on whether the rotation would get through - the server
// answers and the current key still logs in, or the target takes the new key
// by hand - so a long rotation can be tested before it starts.
func (r *Rotator) Check(selector string) (CheckReport, error) {
	profile, keyName, hosts, err := r.profileAndHosts(selector)
	if err != nil {
		return CheckReport{}, err
	}
	rep := CheckReport{Key: profile + "/" + keyName}
	ref := manifest.KeyRef{Profile: profile, KeyName: keyName}
	cur := r.m.KeyPath(r.p.SSHDir, ref)
	if !r.m.Profiles[profile].InOnePassword() && !exists(cur) {
		rep.Problem = "the key is not on disk: run `authama ssh reconcile` first"
		rep.ProblemKind = KindNoKey
		return rep, nil
	}
	for _, h := range hosts {
		prov := r.provider(h)
		hc := HostCheck{Alias: h.Alias, Provider: prov.Name(), OK: true, Via: via(prov)}
		switch prov.Category() {
		case "server":
			st := netcheck.Check(h.Hostname, h.Port, h.RequiresVPN, deref(h.VPNName), deref(h.VPNURL), 4*time.Second, true)
			switch {
			case !st.Reachable:
				hc.OK, hc.Message, hc.Kind = false, st.Message(), KindUnreachable
			case !prov.Verify(r.target(h, profile, cur+".pub", readFile(cur+".pub"), cur)):
				hc.OK = false
				hc.Kind = KindNeverDeployed
				if r.deployedTo(cur, h.Alias) {
					hc.Kind = KindRemoved
					hc.Message = "answers, but the current key no longer logs in: it was removed there, or the user changed; deploy it again first"
				} else {
					hc.Message = "answers, but the current key does not log in: it was never deployed here, so there is nothing to rotate yet; deploy it first (the key's page, Deploy)"
				}
			default:
				hc.Message, hc.Kind = "answers, and the current key logs in", KindLogsIn
			}
		default:
			if providers.TakesKeysByHand(prov) {
				hc.Manual, hc.Kind = true, KindManual
				hc.Message = prov.Name() + " takes the new key by hand: the rotation will stop and ask you to paste it"
			} else {
				hc.Message, hc.Kind = prov.Name()+": the new key goes in through its API", KindAPI
			}
		}
		rep.Hosts = append(rep.Hosts, hc)
	}
	checkPredecessor(r, &rep, ref)
	return rep, nil
}

// checkPredecessor is the dry run of the previous-key policy: where the key
// in old/ still logs in, a rotation that keeps it would stop, and one that
// evicts it would take it off those hosts first.
func checkPredecessor(r *Rotator, rep *CheckReport, ref manifest.KeyRef) {
	live, err := predecessorStillLogsIn(r.p, r.m, r.ks, ref)
	switch {
	case r.EvictOld:
		rep.Evict = live
	case rep.Problem != "":
	case err != nil:
		rep.Problem, rep.ProblemKind = "cannot tell whether the previous key in old/ still logs in somewhere: "+err.Error(), KindOldLive
	case len(live) > 0:
		rep.Problem = "the previous key in old/ still logs in on " + strings.Join(live, ", ") +
			", and the rotation keeps it, so it would stop rather than drop it: take it off there first " +
			"(`authama checklist` lists them), or rotate with --evict-old (--force evicts it unless --keep-old)"
		rep.ProblemKind = KindOldLive
	}
}

// Format is the dry run as lines: the key, then each host with what was found.
func (c CheckReport) Format() string {
	var b strings.Builder
	mark := "ok"
	if !c.OK() {
		mark = "FAIL"
	}
	fmt.Fprintf(&b, "%-5s %s\n", mark, c.Key)
	if c.Problem != "" {
		fmt.Fprintf(&b, "  %-5s %s\n", "FAIL", c.Problem)
	}
	if len(c.Evict) > 0 {
		fmt.Fprintf(&b, "  %-5s would evict old from %d host(s): %s\n", "evict", len(c.Evict), strings.Join(c.Evict, ", "))
	}
	for _, h := range c.Hosts {
		tag := "ok"
		switch {
		case !h.OK:
			tag = "FAIL"
		case h.Manual:
			tag = "paste"
		}
		fmt.Fprintf(&b, "  %-5s %s: %s\n", tag, h.Alias, h.Message)
	}
	return b.String()
}

// via is how a provider takes a new key: ssh for a server, the provider's
// API, or the provider's name where it takes the key by hand.
func via(prov providers.Provider) string {
	switch {
	case prov.Category() == "server":
		return "ssh"
	case providers.TakesKeysByHand(prov):
		return prov.Name()
	}
	return prov.Name() + " api"
}

// deployedTo reports whether the inventory records the key at path as
// deployed to the host alias.
func (r *Rotator) deployedTo(path, alias string) bool {
	if r.inv == nil {
		return false
	}
	for _, rec := range r.inv.Keys {
		if rec.Path != path {
			continue
		}
		for _, d := range rec.Deployments {
			if d.Target == alias {
				return true
			}
		}
	}
	return false
}
