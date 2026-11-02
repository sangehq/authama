package doctor

import (
	"fmt"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/services/vmsync"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// VMStatus is the VM hosts as their hypervisors see them, and what
// `authama ssh vm sync` (and `doctor --fix`) would do about them. It is
// worked out without connecting to any guest.
type VMStatus struct {
	Hosts   []vmsync.Host `json:"hosts"`
	Pending []string      `json:"pending"`
	Notes   []string      `json:"notes"`
	// Gone lists the hosts whose VM the hypervisor no longer has.
	Gone []string `json:"gone"`
	// Undeclared lists the devboxes authama.json declares that are not hosts.
	Undeclared []string `json:"undeclared"`
}

// VMPlan works out the VM status; tests swap it.
var VMPlan = func(p paths.Paths) (vmsync.Plan, bool, error) {
	cfg, _ := settings.LoadVMConfig(p.SettingsFile())
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return vmsync.Plan{}, false, err
	}
	if !hasVMHosts(m) && len(cfg.Devboxes) == 0 {
		return vmsync.Plan{}, false, nil
	}
	opt := vmsync.DefaultOptions(nil)
	opt.Probe = false
	plan, err := vmsync.New(p, cfg, opt).Plan()
	return plan, true, err
}

func hasVMHosts(m *manifest.Manifest) bool {
	for _, pn := range m.ProfileNames() {
		for _, h := range m.Profiles[pn].Hosts {
			if h.VM != nil {
				return true
			}
		}
	}
	return false
}

func (s *Service) vmStatus() *VMStatus {
	plan, any, err := VMPlan(s.p)
	if !any {
		return nil
	}
	st := &VMStatus{Hosts: plan.Hosts, Notes: plan.Notes}
	if err != nil {
		st.Notes = append(st.Notes, "could not ask the hypervisors: "+err.Error())
	}
	for _, h := range plan.Hosts {
		if h.State == "gone" {
			st.Gone = append(st.Gone, fmt.Sprintf("%s/%s (%s %q)", h.Profile, h.Alias, h.Hypervisor, h.VM))
		}
	}
	for _, a := range plan.Actions {
		st.Pending = append(st.Pending, a.Detail)
		if a.Kind == vmsync.ActAddDevbox {
			st.Undeclared = append(st.Undeclared, a.Alias)
		}
	}
	return st
}

// stalePins counts the tagged known_hosts lines no manifest host uses any
// more: what `doctor --fix` and `authama ssh clean` remove.
func (s *Service) stalePins() int {
	if s.m == nil {
		return 0
	}
	c, err := knownhosts.New(s.p.SSHDir).PruneCandidates(s.m)
	if err != nil {
		return 0
	}
	return len(c)
}

// vmLines is the VM section of the human report.
func (r Report) vmLines() []string {
	var lines []string
	if len(r.UnpinnedVMHosts) > 0 {
		lines = append(lines, "VM host keys not pinned yet:")
		for _, h := range r.UnpinnedVMHosts {
			lines = append(lines, "  "+h)
		}
		lines = append(lines, "  -> run: authama ssh vm sync   (pins each one while its VM is running)")
	}
	if r.StalePins > 0 {
		lines = append(lines, fmt.Sprintf("known_hosts: %d stale authama pin(s) for hosts no longer in the manifest "+
			"-> doctor --fix (or authama ssh clean) removes them", r.StalePins))
	}
	if r.VM == nil {
		return lines
	}
	lines = append(lines, "VM hosts:")
	if len(r.VM.Hosts) == 0 {
		lines = append(lines, "  none in the manifest")
	}
	for _, h := range r.VM.Hosts {
		note := ""
		if h.Note != "" {
			note = " - " + h.Note
		}
		lines = append(lines, fmt.Sprintf("  %s/%s  %s %q  %s%s", h.Profile, h.Alias, h.Hypervisor, h.VM, h.State, note))
	}
	if len(r.VM.Pending) > 0 {
		lines = append(lines, "  `authama ssh vm sync` (or doctor --fix) would:")
		for _, a := range r.VM.Pending {
			lines = append(lines, "    - "+a)
		}
	}
	for _, n := range r.VM.Notes {
		lines = append(lines, "  note: "+n)
	}
	return lines
}
