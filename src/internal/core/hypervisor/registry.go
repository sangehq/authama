package hypervisor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/envnames"
)

// CommandSpec declares a hypervisor authama has no built-in driver for: the
// commands that list its VMs and resolve one's address. It is the value of one
// entry in authama.json's "hypervisors" section. Every element is one argv
// element - nothing runs through a shell - and "{vm}" in any element is
// replaced with the VM's name.
//
// list prints the VMs: a JSON array of {"name", "state"} objects (an optional
// boolean "running" overrides state), or one VM per line as name<TAB>state. A
// VM is running when its state is "running", "started" or "up".
//
// address prints where the VM's SSH server answers: host, or host:port.
type CommandSpec struct {
	Description string   `json:"description,omitempty"`
	List        []string `json:"list"`
	Address     []string `json:"address"`
	Start       []string `json:"start,omitempty"`
	Exec        []string `json:"exec,omitempty"`
}

// Validate reports what is wrong with a spec named name.
func (s CommandSpec) Validate(name string) []string {
	var out []string
	if !customNameOK(name) {
		out = append(out, fmt.Sprintf("hypervisors.%s: a custom hypervisor name is lowercase letters, digits, '-' and '_', starting with a letter", name))
	}
	if manifest.IsBuiltinHypervisor(name) {
		out = append(out, fmt.Sprintf("hypervisors.%s: %s is a built-in driver; pick another name", name, name))
	}
	check := func(field string, argv []string, required bool) {
		if len(argv) == 0 {
			if required {
				out = append(out, fmt.Sprintf("hypervisors.%s.%s: required (an argv array, e.g. [\"mytool\", \"list\"])", name, field))
			}
			return
		}
		if strings.TrimSpace(argv[0]) == "" {
			out = append(out, fmt.Sprintf("hypervisors.%s.%s: the first element is the command and must not be empty", name, field))
		}
		for _, a := range argv {
			if strings.ContainsFunc(a, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
				out = append(out, fmt.Sprintf("hypervisors.%s.%s: an element holds a control character", name, field))
				break
			}
		}
	}
	check("list", s.List, true)
	check("address", s.Address, true)
	check("start", s.Start, false)
	check("exec", s.Exec, false)
	return out
}

func customNameOK(name string) bool {
	if name == "" || len(name) > 32 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

var (
	regMu  sync.RWMutex
	custom = map[string]CommandSpec{}
)

// Register replaces the command drivers with specs (authama.json's
// "hypervisors" section) and tells the manifest validator their names.
// Invalid specs are skipped; settings validation reports them.
func Register(specs map[string]CommandSpec) {
	regMu.Lock()
	custom = map[string]CommandSpec{}
	var names []string
	for name, s := range specs {
		if len(s.Validate(name)) > 0 {
			continue
		}
		custom[name] = s
		names = append(names, name)
	}
	regMu.Unlock()
	sort.Strings(names)
	manifest.SetCustomHypervisors(names)
}

func registered() []Driver {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(custom))
	for n := range custom {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Driver, 0, len(names))
	for _, n := range names {
		out = append(out, command{name: n, spec: custom[n]})
	}
	return out
}

// IsCustom reports whether d is a command driver from authama.json.
func IsCustom(d Driver) bool { _, ok := d.(command); return ok }

// Describe is a one-line description of d for listings.
func Describe(d Driver) string {
	if c, ok := d.(command); ok {
		if c.spec.Description != "" {
			return "custom: " + c.spec.Description
		}
		return "custom command driver"
	}
	return "built-in"
}

// EnabledNames parses $AUTHAMA_VM_HYPERVISORS (authama.json vm.hypervisors):
// the hypervisors the user has and uses, comma- or space-separated. nil means
// the setting is unset, so every driver is enabled.
func EnabledNames(get func(string) string) []string {
	raw := strings.TrimSpace(envnames.Get(get, envnames.VMHypervisors))
	if raw == "" {
		return nil
	}
	var out []string
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		out = append(out, strings.ToLower(f))
	}
	return out
}

// Enabled returns the drivers the user enabled, in All's order; every driver
// when the setting is unset.
func Enabled() []Driver {
	names := EnabledNames(nil)
	if names == nil {
		return All()
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	var out []Driver
	for _, d := range All() {
		if want[d.Name()] {
			out = append(out, d)
		}
	}
	return out
}

// IsEnabled reports whether the hypervisor called name is enabled.
func IsEnabled(name string) bool {
	names := EnabledNames(nil)
	if names == nil {
		return true
	}
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

// UnknownEnabled lists names in the enabled setting that match no driver.
func UnknownEnabled() []string {
	known := map[string]bool{}
	for _, d := range All() {
		known[d.Name()] = true
	}
	var out []string
	for _, n := range EnabledNames(nil) {
		if !known[n] {
			out = append(out, n)
		}
	}
	return out
}

// Exister is a driver that can say a VM it does not list still exists (VMware
// lists running VMs only).
type Exister interface {
	Exists(vm string) (bool, error)
}

// Exists: a VMware VM is its .vmx file.
func (vmware) Exists(vm string) (bool, error) {
	_, err := os.Stat(vm)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// --- the command driver ------------------------------------------------------------------------

type command struct {
	name string
	spec CommandSpec
}

func (c command) Name() string { return c.name }

// CLI is the list command's executable, resolved through PATH.
func (c command) CLI() string { return lookup(c.spec.List[0]) }

func expand(argv []string, vm string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		out[i] = strings.ReplaceAll(a, "{vm}", vm)
	}
	return out
}

func (c command) runArgv(argv []string, vm string, extra ...string) ([]byte, error) {
	full := append(expand(argv, vm), extra...)
	cli := lookup(full[0])
	if cli == "" {
		return nil, fmt.Errorf("%s: %s not found on PATH", c.name, full[0])
	}
	return run(cli, slow, full[1:]...)
}

func runningState(state string) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "started", "up":
		return true
	}
	return false
}

// ParseCommandList reads a command driver's list output (exported for tests
// and for `vm list` diagnostics).
func ParseCommandList(out []byte) ([]VM, error) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var raw []struct {
			Name    string `json:"name"`
			State   string `json:"state"`
			Running *bool  `json:"running"`
		}
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("list printed JSON authama cannot read: %w", err)
		}
		var vms []VM
		for _, v := range raw {
			if v.Name == "" {
				continue
			}
			run := runningState(v.State)
			if v.Running != nil {
				run = *v.Running
			}
			st := v.State
			if st == "" {
				st = map[bool]string{true: "running", false: "stopped"}[run]
			}
			vms = append(vms, VM{Name: v.Name, State: st, Running: run})
		}
		return vms, nil
	}
	var vms []VM
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, state, _ := strings.Cut(line, "\t")
		name, state = strings.TrimSpace(name), strings.TrimSpace(state)
		if state == "" {
			state = "unknown"
		}
		vms = append(vms, VM{Name: name, State: state, Running: runningState(state)})
	}
	return vms, nil
}

func (c command) List() ([]VM, error) {
	out, err := c.runArgv(c.spec.List, "")
	if err != nil {
		return nil, err
	}
	return ParseCommandList(out)
}

// ParseAddress reads "host" or "host:port" (the first non-empty line).
func ParseAddress(out []byte) (Endpoint, error) {
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if h, p, err := net.SplitHostPort(line); err == nil {
			port, perr := strconv.Atoi(p)
			if perr != nil || port < 1 || port > 65535 {
				return Endpoint{}, fmt.Errorf("address %q: bad port", line)
			}
			return Endpoint{Host: h, Port: port}, nil
		}
		if strings.ContainsAny(line, " \t") {
			return Endpoint{}, fmt.Errorf("address %q is not a host or host:port", line)
		}
		return Endpoint{Host: strings.Trim(line, "[]"), Port: 22}, nil
	}
	return Endpoint{}, errors.New("address printed nothing")
}

func (c command) Address(vm string) (Endpoint, error) {
	out, err := c.runArgv(c.spec.Address, vm)
	if err != nil {
		return Endpoint{}, err
	}
	return ParseAddress(out)
}

func (c command) Start(vm string) error {
	if len(c.spec.Start) == 0 {
		return ErrNotSupported
	}
	_, err := c.runArgv(c.spec.Start, vm)
	return err
}

func (c command) Exec(vm string, argv ...string) ([]byte, error) {
	if len(c.spec.Exec) == 0 {
		return nil, ErrNotSupported
	}
	return c.runArgv(c.spec.Exec, vm, argv...)
}

// CanExec reports whether d can run a command in a guest without SSH (and so
// re-install a key). A driver may say so itself with a CanExec() bool method.
func CanExec(d Driver) bool {
	switch x := d.(type) {
	case vmware, orbstack, podman:
		return false
	case command:
		return len(x.spec.Exec) > 0
	}
	if c, ok := d.(interface{ CanExec() bool }); ok {
		return c.CanExec()
	}
	return true
}
