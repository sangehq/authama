// Package vmsync keeps a manifest's VM hosts in step with the hypervisors
// that run them: `authama ssh vm sync`, the VM half of `authama doctor --fix`
// and the web UI's VM page.
//
// For every VM host it asks the hypervisor what became of the VM, and plans:
//
//   - a running VM: pin its host key under the alias when nothing is pinned
//     (replace a changed pin only when asked to), re-install the host's
//     public key in the guest when the guest refuses it, and create or
//     refresh the Docker context ssh://<alias>;
//   - a VM the hypervisor no longer has: remove the host entry (never its key
//     files) and the Docker context authama made for it;
//   - a suspended or stopped VM: report it, and leave it alone - sync never
//     starts, stops or deletes a VM.
//
// It also adds the devboxes authama.json declares that are not hosts yet
// (adopting a declared loose key by copying it) and removes Docker contexts
// authama made whose host is gone from the manifest.
//
// A plan is computed without writing anything, so a dry run shows exactly
// what a run does; a run applied twice changes nothing the second time.
// Nothing here ever reads a private key: the guest gets the .pub.
package vmsync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/simtabi/authama/src/internal/core/hypervisor"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/dockerctx"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Action kinds.
const (
	ActPin          = "pin"           // pin the guest's host key under the alias
	ActRepin        = "repin"         // replace a changed pin (opt-in)
	ActInstallKey   = "install-key"   // append the host's .pub to the guest's authorized_keys
	ActDockerCreate = "docker-create" // docker context create <alias>
	ActDockerUpdate = "docker-update" // docker context update <alias>
	ActDockerRemove = "docker-remove" // docker context rm <alias> (backed up first)
	ActRemoveHost   = "remove-host"   // drop a gone VM's host entry (keys stay)
	ActAddDevbox    = "add-devbox"    // add a devbox authama.json declares
	ActAdoptKey     = "adopt-key"     // copy a declared loose key into the profile
	ActUpdateDevbox = "update-devbox" // bring a host in line with its declaration
)

// Action is one step a sync takes.
type Action struct {
	Kind    string
	Profile string
	Alias   string
	Detail  string // the line a plan prints

	// What the step needs to run.
	driver  hypervisor.Driver
	vm      string
	user    string
	pubLine string
	token   string
	scanned []knownhosts.ScannedKey
	context dockerctx.Context
	devbox  settings.Devbox
	keySrc  string // adopt-key: the loose private key path
	keyName string
}

// Outside reports whether the action acts outside the manifest and config
// home (the guest, known_hosts, Docker).
func (a Action) Outside() bool {
	switch a.Kind {
	case ActRemoveHost, ActAddDevbox, ActAdoptKey, ActUpdateDevbox:
		return false
	}
	return true
}

// Host is one VM host as the hypervisor sees it now.
type Host struct {
	Profile    string `json:"profile"`
	Alias      string `json:"alias"`
	Hypervisor string `json:"hypervisor"`
	VM         string `json:"vm"`
	// State: running | stopped | suspended | ... (the hypervisor's word) |
	// gone | unknown | disabled | not-installed.
	State string `json:"state"`
	Note  string `json:"note,omitempty"`
	// Declared is set when authama.json's devboxes section declares the host.
	Declared bool `json:"declared,omitempty"`
}

// Plan is everything a sync would do, and why.
type Plan struct {
	Hosts   []Host   `json:"hosts"`
	Actions []Action `json:"-"`
	Notes   []string `json:"notes,omitempty"`
}

// Lines are the plan's actions as printable lines.
func (p Plan) Lines() []string {
	out := make([]string, 0, len(p.Actions))
	for _, a := range p.Actions {
		out = append(out, a.Detail)
	}
	return out
}

// Empty reports whether the plan changes nothing.
func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// Options steer a sync.
type Options struct {
	// Probe connects to running guests (ssh-keyscan and one BatchMode ssh)
	// to check the host key and the key. Off, the plan is the hypervisor's
	// view only - what doctor shows without --fix.
	Probe bool
	// Repin replaces a VM host's pin when the guest's host key changed.
	Repin bool
	// PruneGone removes the host of a VM the hypervisor no longer has.
	PruneGone bool
	// AutoPin pins an unpinned running VM host (ssh.auto_pin).
	AutoPin bool
	// Docker manages Docker contexts (off when the docker CLI is missing).
	Docker bool
}

// DefaultOptions reads the settings: vm.prune_gone, vm.repin_changed,
// ssh.auto_pin, and whether docker is installed.
func DefaultOptions(getenv func(string) string) Options {
	if getenv == nil {
		getenv = os.Getenv
	}
	flag := func(name string, def bool) bool {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			return def
		}
		return settings.Truthy(v)
	}
	return Options{
		Probe:     true,
		Repin:     flag("AUTHAMA_VM_REPIN", false),
		PruneGone: flag("AUTHAMA_VM_PRUNE_GONE", true),
		AutoPin:   flag("AUTHAMA_AUTO_PIN", true),
		Docker:    dockerctx.Available() == nil,
	}
}

// Deps are the outside world, swappable in tests.
type Deps struct {
	Driver  func(name string) (hypervisor.Driver, error)
	Enabled func(name string) bool
	Scan    func(sshDir, host string, port int) ([]knownhosts.ScannedKey, error)
	Probe   func(sshDir, alias string, port int, scanned []knownhosts.ScannedKey) ProbeResult
	Docker  DockerOps
}

// DockerOps is the docker CLI.
type DockerOps interface {
	List() ([]dockerctx.Context, error)
	Apply(alias string) (string, error)
	Remove(c dockerctx.Context, backupDir string) (string, error)
}

type realDocker struct{}

func (realDocker) List() ([]dockerctx.Context, error) { return dockerctx.List() }
func (realDocker) Apply(a string) (string, error)     { return dockerctx.Apply(a) }
func (realDocker) Remove(c dockerctx.Context, d string) (string, error) {
	return dockerctx.Remove(c, d)
}

// RealDeps is the real hypervisors, ssh and docker.
func RealDeps() Deps {
	return Deps{
		Driver:  hypervisor.For,
		Enabled: hypervisor.IsEnabled,
		Scan: func(sshDir, host string, port int) ([]knownhosts.ScannedKey, error) {
			return knownhosts.New(sshDir).ScanChecked(host, port)
		},
		Probe:  sshProbe,
		Docker: realDocker{},
	}
}

// Syncer plans and applies a sync.
type Syncer struct {
	P    paths.Paths
	Cfg  settings.VMConfig
	Opt  Options
	Deps Deps
}

// New builds a Syncer over the real world.
func New(p paths.Paths, cfg settings.VMConfig, opt Options) *Syncer {
	return &Syncer{P: p, Cfg: cfg, Opt: opt, Deps: RealDeps()}
}

type listing struct {
	vms []hypervisor.VM
	err error
}

// Plan works out what a sync would do. It writes nothing.
func (s *Syncer) Plan() (Plan, error) {
	var plan Plan
	m, err := manifest.Load(s.P.Manifest())
	if err != nil {
		return plan, err
	}
	var contexts map[string]dockerctx.Context
	if s.Opt.Docker {
		cs, err := s.Deps.Docker.List()
		if err != nil {
			plan.Notes = append(plan.Notes, "docker: "+err.Error()+"; Docker contexts left as they are")
			s.Opt.Docker = false
		} else {
			contexts = map[string]dockerctx.Context{}
			for _, c := range cs {
				contexts[c.Name] = c
			}
		}
	}
	lists := map[string]listing{}
	list := func(d hypervisor.Driver) listing {
		if l, ok := lists[d.Name()]; ok {
			return l
		}
		vms, err := d.List()
		l := listing{vms, err}
		lists[d.Name()] = l
		return l
	}
	allAliases := map[string]bool{}
	for _, pname := range m.ProfileNames() {
		for _, h := range m.Profiles[pname].Hosts {
			allAliases[h.Alias] = true
			if h.VM == nil {
				continue
			}
			s.planHost(&plan, m, pname, h, list, contexts)
		}
	}
	s.planDevboxes(&plan, m, allAliases)
	// A context authama made whose host is gone from the manifest entirely.
	if s.Opt.Docker {
		names := make([]string, 0, len(contexts))
		for n := range contexts {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			c := contexts[n]
			if allAliases[n] || c.Description != dockerctx.Marker || c.Endpoint != dockerctx.Endpoint(n) || s.planned(plan, ActDockerRemove, n) {
				continue
			}
			if _, declared := s.Cfg.Devboxes[n]; declared {
				continue
			}
			if c.Current {
				plan.Notes = append(plan.Notes, fmt.Sprintf("docker context %s points at a host that no longer exists, but it is the current context; switch away (docker context use default) and sync again", n))
				continue
			}
			plan.Actions = append(plan.Actions, Action{Kind: ActDockerRemove, Alias: n, context: c,
				Detail: fmt.Sprintf("remove the Docker context %s: no host %s is left (exported to a backup first)", n, n)})
		}
	}
	return plan, nil
}

func (s *Syncer) planned(plan Plan, kind, alias string) bool {
	for _, a := range plan.Actions {
		if a.Kind == kind && a.Alias == alias {
			return true
		}
	}
	return false
}

func (s *Syncer) planHost(plan *Plan, m *manifest.Manifest, pname string, h manifest.Host,
	list func(hypervisor.Driver) listing, contexts map[string]dockerctx.Context) {
	st := Host{Profile: pname, Alias: h.Alias, Hypervisor: h.VM.Hypervisor, VM: h.VM.Name}
	_, st.Declared = s.Cfg.Devboxes[h.Alias]
	add := func(state, note string) {
		st.State, st.Note = state, note
		plan.Hosts = append(plan.Hosts, st)
	}
	if !s.Deps.Enabled(h.VM.Hypervisor) {
		add("disabled", h.VM.Hypervisor+" is not in vm.hypervisors; left alone")
		return
	}
	d, err := s.Deps.Driver(h.VM.Hypervisor)
	if err != nil {
		add("unknown", err.Error()+"; left alone")
		return
	}
	if !hypervisor.Available(d) {
		add("not-installed", d.Name()+"'s CLI is not installed; left alone")
		return
	}
	l := list(d)
	if l.err != nil {
		add("unknown", "could not list "+d.Name()+" VMs ("+l.err.Error()+"); left alone")
		return
	}
	var vm *hypervisor.VM
	for i := range l.vms {
		if l.vms[i].Matches(h.VM.Name) {
			vm = &l.vms[i]
			break
		}
	}
	if vm == nil {
		if ex, ok := d.(hypervisor.Exister); ok {
			if there, err := ex.Exists(h.VM.Name); err != nil || there {
				add("stopped", "not running (VMware lists running VMs only); left alone")
				return
			}
		} else if len(l.vms) == 0 {
			add("unknown", d.Name()+" lists no VMs at all, so authama will not call this one gone; "+
				"if it is, remove it with: authama ssh host delete "+pname+" "+h.Alias)
			return
		}
		if st.Declared {
			add("gone", d.Name()+" has no VM "+fmt.Sprintf("%q", h.VM.Name)+"; the host is declared in authama.json's devboxes, so it stays - fix or remove the declaration")
			return
		}
		if !s.Opt.PruneGone {
			add("gone", "the VM is gone; vm.prune_gone is off, so the host stays (authama ssh host delete "+pname+" "+h.Alias+")")
			return
		}
		add("gone", "the VM is gone: the host entry is removed; its key files are kept")
		plan.Actions = append(plan.Actions, Action{Kind: ActRemoveHost, Profile: pname, Alias: h.Alias,
			Detail: fmt.Sprintf("remove host %s/%s: %s has no VM %q any more (key files kept)", pname, h.Alias, d.Name(), h.VM.Name)})
		if c, ok := contexts[h.Alias]; ok && c.Managed() {
			if c.Current {
				plan.Notes = append(plan.Notes, fmt.Sprintf("docker context %s is the current context; it stays until you switch away (docker context use default)", h.Alias))
			} else {
				plan.Actions = append(plan.Actions, Action{Kind: ActDockerRemove, Alias: h.Alias, context: c,
					Detail: fmt.Sprintf("remove the Docker context %s (exported to a backup first)", h.Alias)})
			}
		}
		return
	}
	if !vm.Running {
		add(vm.State, "not running; left alone (sync never starts, stops or deletes a VM)")
		return
	}
	st.State = vm.State
	if !s.Opt.Probe {
		plan.Hosts = append(plan.Hosts, st)
		return
	}
	notes := s.planRunning(plan, m, pname, h, d, *vm, contexts)
	st.Note = strings.Join(notes, "; ")
	plan.Hosts = append(plan.Hosts, st)
}

// planRunning plans the pin, the key and the Docker context of a running VM.
func (s *Syncer) planRunning(plan *Plan, m *manifest.Manifest, pname string, h manifest.Host,
	d hypervisor.Driver, vm hypervisor.VM, contexts map[string]dockerctx.Context) []string {
	var notes []string
	ep, err := d.Address(h.VM.Name)
	if err != nil {
		return []string{"no address yet (" + err.Error() + ")"}
	}
	port := ep.Port
	if h.Port != 0 && h.Port != 22 {
		port = h.Port
	}
	scanned, err := s.Deps.Scan(s.P.SSHDir, ep.Host, port)
	if err != nil {
		return []string{"the guest's SSH server is not answering (" + err.Error() + ")"}
	}
	token := knownhosts.HostToken(h.Alias, h.Port)
	kh := knownhosts.New(s.P.SSHDir)
	switch kh.PinState(token, scanned) {
	case knownhosts.PinNone:
		if s.Opt.AutoPin {
			plan.Actions = append(plan.Actions, Action{Kind: ActPin, Profile: pname, Alias: h.Alias, token: token, scanned: scanned,
				Detail: fmt.Sprintf("pin %s's host key under %s (%s)", h.Alias, token, fingerprints(scanned))})
		} else {
			notes = append(notes, "host key not pinned (ssh.auto_pin is off)")
		}
	case knownhosts.PinChanged:
		if s.Opt.Repin {
			plan.Actions = append(plan.Actions, Action{Kind: ActRepin, Profile: pname, Alias: h.Alias, token: token, scanned: scanned,
				Detail: fmt.Sprintf("replace %s's pinned host key: the guest now offers %s", h.Alias, fingerprints(scanned))})
		} else {
			notes = append(notes, "the guest's host key changed since it was pinned (a rebuilt VM?); "+
				"if you expected that, run: authama ssh vm sync --repin")
		}
	}
	pr := s.Deps.Probe(s.P.SSHDir, h.Alias, h.Port, scanned)
	switch pr.Status {
	case ProbeKeyRefused:
		if a, note := s.planKeyInstall(m, pname, h, d, vm); a != nil {
			plan.Actions = append(plan.Actions, *a)
		} else {
			notes = append(notes, note)
		}
	case ProbeUnreachable:
		notes = append(notes, "ssh did not get through ("+pr.Detail+")")
	case ProbeOK:
		notes = append(notes, "key accepted")
	}
	if s.Opt.Docker {
		s.planDocker(plan, h.Alias, pr, contexts)
	}
	return notes
}

func (s *Syncer) planDocker(plan *Plan, alias string, pr ProbeResult, contexts map[string]dockerctx.Context) {
	if db, ok := s.Cfg.Devboxes[alias]; ok && !db.WantsDockerContext() {
		return
	}
	c, have := contexts[alias]
	switch {
	case have && c.Endpoint == dockerctx.Endpoint(alias) && c.Description == dockerctx.Marker:
		// up to date
	case have && c.Endpoint != dockerctx.Endpoint(alias) && c.Description != dockerctx.Marker:
		plan.Notes = append(plan.Notes, fmt.Sprintf("docker context %s exists but points at %s, and authama did not make it; left alone", alias, c.Endpoint))
	case have:
		plan.Actions = append(plan.Actions, Action{Kind: ActDockerUpdate, Alias: alias,
			Detail: fmt.Sprintf("refresh the Docker context %s -> ssh://%s", alias, alias)})
	case pr.Docker:
		plan.Actions = append(plan.Actions, Action{Kind: ActDockerCreate, Alias: alias,
			Detail: fmt.Sprintf("create the Docker context %s -> ssh://%s (the guest runs Docker)", alias, alias)})
	}
}

func (s *Syncer) planKeyInstall(m *manifest.Manifest, pname string, h manifest.Host, d hypervisor.Driver, vm hypervisor.VM) (*Action, string) {
	keyName, err := m.ResolvedKeyName(pname, h)
	if err != nil {
		return nil, "the guest refuses the key, and its name does not resolve: " + err.Error()
	}
	pub := m.KeyPath(s.P.SSHDir, manifest.KeyRef{Profile: pname, KeyName: keyName}) + ".pub"
	line, err := readPubLine(pub)
	if err != nil {
		return nil, "the guest refuses the key, and " + keyName + ".pub is not there to install (run: authama ssh reconcile)"
	}
	if !settings.ValidUser(h.User) {
		return nil, fmt.Sprintf("the guest refuses the key; user %q is not one authama will script for - install %s by hand", h.User, pub)
	}
	manual := fmt.Sprintf("install it by hand: append %s to ~%s/.ssh/authorized_keys in the guest", pub, h.User)
	if vm.Guest == "windows" {
		return nil, "the guest refuses the key and runs Windows, which authama does not script; " +
			fmt.Sprintf("append %s to C:\\Users\\%s\\.ssh\\authorized_keys (an administrator: C:\\ProgramData\\ssh\\administrators_authorized_keys)", pub, h.User)
	}
	if !hypervisor.CanExec(d) {
		return nil, "the guest refuses the key, and " + d.Name() + " cannot run commands in it; " + manual
	}
	return &Action{Kind: ActInstallKey, Profile: pname, Alias: h.Alias, driver: d, vm: h.VM.Name, user: h.User, pubLine: line,
		Detail: fmt.Sprintf("install %s.pub in %s's guest (%s %q, user %s): the guest refuses the key", keyName, h.Alias, d.Name(), h.VM.Name, h.User)}, ""
}

// planDevboxes plans the declared devboxes that are not hosts, or differ.
func (s *Syncer) planDevboxes(plan *Plan, m *manifest.Manifest, aliases map[string]bool) {
	for _, alias := range s.Cfg.DevboxAliases() {
		db := s.Cfg.Devboxes[alias]
		if aliases[alias] {
			s.planDevboxDrift(plan, m, alias, db)
			continue
		}
		plan.Hosts = append(plan.Hosts, Host{Profile: db.Profile, Alias: alias, Hypervisor: db.Hypervisor, VM: db.VM,
			State: "declared", Note: "declared in authama.json, not a host yet", Declared: true})
		prof := ""
		if _, ok := m.Profiles[db.Profile]; !ok {
			prof = " (creating profile " + db.Profile + ")"
		}
		plan.Actions = append(plan.Actions, Action{Kind: ActAddDevbox, Profile: db.Profile, Alias: alias, devbox: db,
			Detail: fmt.Sprintf("add the declared devbox %s to %s%s: %s %q, user %s", alias, db.Profile, prof, db.Hypervisor, db.VM, db.User)})
		if db.Key == "" {
			continue
		}
		dst := m.KeyPath(s.P.SSHDir, manifest.KeyRef{Profile: db.Profile, KeyName: db.Key})
		if fileExists(dst) {
			continue
		}
		src := filepath.Join(s.P.SSHDir, db.Key)
		if fileExists(src) && fileExists(src+".pub") {
			plan.Actions = append(plan.Actions, Action{Kind: ActAdoptKey, Profile: db.Profile, Alias: alias, keySrc: src, keyName: db.Key,
				Detail: fmt.Sprintf("adopt ~/.ssh/%s into profile %s (a copy; the original stays)", db.Key, db.Profile)})
		} else {
			plan.Notes = append(plan.Notes, fmt.Sprintf("devbox %s names key %s, which is neither in profile %s nor a loose pair in ~/.ssh; `authama ssh reconcile` will mint it", alias, db.Key, db.Profile))
		}
	}
}

func (s *Syncer) planDevboxDrift(plan *Plan, m *manifest.Manifest, alias string, db settings.Devbox) {
	for _, pname := range m.ProfileNames() {
		for _, h := range m.Profiles[pname].Hosts {
			if h.Alias != alias {
				continue
			}
			if pname != db.Profile {
				plan.Notes = append(plan.Notes, fmt.Sprintf("devbox %s is declared in profile %s but the host is in %s; move it by hand", alias, db.Profile, pname))
				return
			}
			var diffs []string
			if h.VM == nil || h.VM.Hypervisor != db.Hypervisor || h.VM.Name != db.VM {
				diffs = append(diffs, fmt.Sprintf("vm %s %q", db.Hypervisor, db.VM))
			}
			if h.User != db.User {
				diffs = append(diffs, "user "+db.User)
			}
			if db.Key != "" && (h.KeyName == nil || *h.KeyName != db.Key) && m.Profiles[pname].KeyScope != "shared" {
				diffs = append(diffs, "key "+db.Key)
			}
			if len(diffs) > 0 {
				plan.Actions = append(plan.Actions, Action{Kind: ActUpdateDevbox, Profile: pname, Alias: alias, devbox: db,
					Detail: fmt.Sprintf("update host %s/%s to its declaration: %s", pname, alias, strings.Join(diffs, ", "))})
			}
			return
		}
	}
}

func fingerprints(keys []knownhosts.ScannedKey) string {
	var out []string
	for _, k := range keys {
		out = append(out, k.Keytype+" "+k.Fingerprint)
	}
	return strings.Join(out, ", ")
}

func readPubLine(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	f := strings.Fields(line)
	if len(f) < 2 || !strings.HasPrefix(f[0], "ssh-") && !strings.HasPrefix(f[0], "ecdsa-") && !strings.HasPrefix(f[0], "sk-") {
		return "", fmt.Errorf("%s is not an OpenSSH public key", path)
	}
	// The comment is dropped when it holds a quote: the line is embedded in a
	// single-quoted script.
	if strings.ContainsAny(line, "'\\") {
		line = f[0] + " " + f[1]
	}
	return line, nil
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
