package web

import (
	"sort"

	"github.com/simtabi/authama/src/internal/core/hypervisor"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/keyaudit"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/services/vmsync"
	"github.com/simtabi/authama/src/internal/util/scheduler"
)

// VMsView is the /vms page: the VM hosts as their hypervisors see them, what
// a sync would do (worked out without connecting to a guest), the drivers,
// the declared devboxes, and the loose keys that could be adopted.
type VMsView struct {
	Hosts    []vmsync.Host
	Pending  []string
	Notes    []string
	Drivers  []DriverView
	Unknown  []string // enabled names that match no driver
	Devboxes []DevboxView
	Loose    []keyaudit.LooseKey
	Profiles []string
	// Schedule is the command the daily schedule runs.
	Schedule string
}

// DriverView is one hypervisor driver and the VMs it lists.
type DriverView struct {
	Name, Kind, CLI string
	Enabled         bool
	VMs             []hypervisor.VM
	Err             string
}

// DevboxView is one devbox authama.json declares.
type DevboxView struct {
	Alias string
	settings.Devbox
}

// VMs builds the /vms page. It asks the hypervisors but connects to no guest.
func (e AppEngine) VMs() (VMsView, error) {
	var v VMsView
	cfg, _ := settings.LoadVMConfig(e.P.SettingsFile())
	opt := vmsync.DefaultOptions(nil)
	opt.Probe = false
	if plan, err := vmsync.New(e.P, cfg, opt).Plan(); err != nil {
		v.Notes = append(v.Notes, "could not plan a sync: "+err.Error())
	} else {
		v.Hosts, v.Pending, v.Notes = plan.Hosts, plan.Lines(), plan.Notes
	}
	for _, d := range hypervisor.All() {
		dv := DriverView{Name: d.Name(), Kind: hypervisor.Describe(d), CLI: d.CLI(), Enabled: hypervisor.IsEnabled(d.Name())}
		if dv.Enabled && dv.CLI != "" {
			vms, err := d.List()
			if err != nil {
				dv.Err = err.Error()
			}
			dv.VMs = vms
		}
		v.Drivers = append(v.Drivers, dv)
	}
	v.Unknown = hypervisor.UnknownEnabled()
	for _, a := range cfg.DevboxAliases() {
		v.Devboxes = append(v.Devboxes, DevboxView{Alias: a, Devbox: cfg.Devboxes[a]})
	}
	for _, k := range keyaudit.LooseKeys(e.P.SSHDir) {
		if k.CopyOf == "" {
			v.Loose = append(v.Loose, k)
		}
	}
	if m, err := manifest.Load(e.P.Manifest()); err == nil {
		v.Profiles = m.ProfileNames()
		sort.Strings(v.Profiles)
	}
	v.Schedule = scheduler.VMSyncCommand()
	return v, nil
}
