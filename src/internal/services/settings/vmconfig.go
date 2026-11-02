package settings

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/simtabi/authama/src/internal/core/hypervisor"
	"github.com/simtabi/authama/src/internal/core/manifest"
)

// The structured sections: JSON-only settings with no environment variable.
const (
	SectionHypervisors = "hypervisors"
	SectionDevboxes    = "devboxes"
)

var (
	structuredNames = []string{SectionHypervisors, SectionDevboxes}
	structured      = map[string]bool{SectionHypervisors: true, SectionDevboxes: true}
)

// Devbox declares one VM authama should keep as an SSH host: which hypervisor
// runs it, the VM's name there, the guest user and the key. `ssh vm sync`
// adds a declared devbox that is not a host yet, and adopts a declared loose
// key (copying it, never moving it) when the profile has no key of that name.
type Devbox struct {
	Profile    string `json:"profile"`
	Hypervisor string `json:"hypervisor"`
	VM         string `json:"vm"`
	User       string `json:"user"`
	// Key is the key name the host uses. When the profile has no such key and
	// ~/.ssh/<key> is a loose keypair, sync adopts that pair into the profile.
	Key string `json:"key,omitempty"`
	// DockerContext keeps a Docker context ssh://<alias> (default true).
	DockerContext *bool  `json:"docker_context,omitempty"`
	Description   string `json:"description,omitempty"`
}

// Planned reports a devbox declared without a VM yet: it only keeps its key
// (reserved, so doctor does not call it loose) until the VM exists.
func (d Devbox) Planned() bool { return strings.TrimSpace(d.VM) == "" }

// PlannedKeys are the keys planned devboxes keep, as reservations keyed by
// the ~/.ssh file name, For naming the devbox.
func (c VMConfig) PlannedKeys() map[string]manifest.ReservedKey {
	out := map[string]manifest.ReservedKey{}
	for _, alias := range c.DevboxAliases() {
		d := c.Devboxes[alias]
		if d.Planned() && d.Key != "" {
			out[d.Key] = manifest.ReservedKey{For: "devbox " + alias, Note: d.Description}
		}
	}
	return out
}

// WantsDockerContext is DockerContext, defaulting to true.
func (d Devbox) WantsDockerContext() bool { return d.DockerContext == nil || *d.DockerContext }

// VMConfig is the "hypervisors" and "devboxes" sections.
type VMConfig struct {
	Hypervisors map[string]hypervisor.CommandSpec `json:"hypervisors,omitempty"`
	Devboxes    map[string]Devbox                 `json:"devboxes,omitempty"`
}

// DevboxAliases returns the declared aliases, sorted.
func (c VMConfig) DevboxAliases() []string {
	out := make([]string, 0, len(c.Devboxes))
	for a := range c.Devboxes {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

var (
	aliasRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
	userRE  = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,31}$`)
	nameRE  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

// ValidUser reports whether u is a guest user name authama will embed in the
// key-install script (letters, digits, '.', '_' and '-').
func ValidUser(u string) bool { return userRE.MatchString(u) }

func parseStructured(f *File, name string, raw json.RawMessage) []string {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var probs []string
	switch name {
	case SectionHypervisors:
		var m map[string]hypervisor.CommandSpec
		if err := d.Decode(&m); err != nil || m == nil {
			return []string{"hypervisors: must be an object of name -> {list, address, start?, exec?, description?}: " + errText(err)}
		}
		for _, n := range sortedSpecKeys(m) {
			probs = append(probs, m[n].Validate(n)...)
		}
		f.VM.Hypervisors = m
	case SectionDevboxes:
		var m map[string]Devbox
		if err := d.Decode(&m); err != nil || m == nil {
			return []string{"devboxes: must be an object of alias -> {profile, hypervisor, vm, user, key?, docker_context?} (or {profile, key} for a planned devbox with no VM yet): " + errText(err)}
		}
		f.VM.Devboxes = m
	}
	return probs
}

func errText(err error) string {
	if err == nil {
		return "got null"
	}
	return err.Error()
}

func sortedSpecKeys(m map[string]hypervisor.CommandSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// crossCheck validates the devboxes against the built-in and declared
// hypervisors.
func (c VMConfig) crossCheck() []string {
	var probs []string
	for _, alias := range c.DevboxAliases() {
		d := c.Devboxes[alias]
		at := "devboxes." + alias
		if !aliasRE.MatchString(alias) {
			probs = append(probs, at+": an alias is letters, digits, '.', '_' and '-'")
		}
		if !nameRE.MatchString(d.Profile) {
			probs = append(probs, at+".profile: required (a profile name)")
		}
		// A devbox without a vm is planned: it holds its key for a VM that does
		// not exist yet (doctor reports the key as reserved, sync adds no host),
		// so only the key is required; hypervisor and user are checked if given.
		planned := d.Planned()
		switch {
		case d.Hypervisor == "" && planned:
		case d.Hypervisor == "":
			probs = append(probs, at+".hypervisor: required")
		case d.Hypervisor == manifest.HypervisorOrbStack || d.Hypervisor == manifest.HypervisorPodman:
			probs = append(probs, at+".hypervisor: "+d.Hypervisor+" runs its own SSH keys, so it cannot be a devbox host (use `authama ssh vm docker-context`)")
		case !manifest.IsBuiltinHypervisor(d.Hypervisor) && c.Hypervisors[d.Hypervisor].List == nil:
			probs = append(probs, fmt.Sprintf("%s.hypervisor: %q is neither built in (%s) nor declared in the hypervisors section",
				at, d.Hypervisor, strings.Join(manifest.Hypervisors, ", ")))
		}
		switch {
		case planned && d.Key == "":
			probs = append(probs, at+".vm: required (the VM's name as the hypervisor lists it), "+
				"or leave it out and set key to keep that key for a VM you have not made yet")
		case strings.ContainsFunc(d.VM, func(r rune) bool { return r < 0x20 || r == 0x7f }):
			probs = append(probs, at+".vm: required (the VM's name as the hypervisor lists it)")
		}
		if !ValidUser(d.User) && (!planned || d.User != "") {
			probs = append(probs, at+".user: required (the guest user: letters, digits, '.', '_' and '-')")
		}
		if d.Key != "" && !nameRE.MatchString(d.Key) {
			probs = append(probs, at+".key: a key name, not a path (letters, digits, '.', '_' and '-')")
		}
	}
	return probs
}

// structuredSchema is the JSON Schema of the structured sections.
func structuredSchema() map[string]any {
	type obj = map[string]any
	argv := func(doc string) obj {
		return obj{"type": "array", "minItems": 1, "items": obj{"type": "string"}, "description": doc}
	}
	return obj{
		SectionHypervisors: obj{
			"type":        "object",
			"description": "Custom hypervisors: a command driver per name, for a VM tool authama has no built-in driver for. Each value is an argv array (no shell); {vm} is replaced with the VM's name. Enable one by naming it in vm.hypervisors (or leave that unset).",
			"propertyNames": obj{"pattern": "^[a-z][a-z0-9_-]{0,31}$",
				"not": obj{"enum": manifest.Hypervisors}},
			"additionalProperties": obj{
				"type": "object", "additionalProperties": false, "required": []string{"list", "address"},
				"properties": obj{
					"description": obj{"type": "string", "description": "Shown by `ssh vm list`."},
					"list":        argv("Lists the VMs: a JSON array of {\"name\",\"state\"} (optional boolean \"running\"), or one name<TAB>state per line. running, started and up mean running."),
					"address":     argv("Prints where the VM's SSH server answers: host or host:port."),
					"start":       argv("Starts the VM (optional; without it `ssh` to a stopped VM fails instead of starting it)."),
					"exec":        argv("Runs a command in the guest; the guest argv is appended (optional; lets `ssh vm sync` re-install a missing key)."),
				},
			},
		},
		SectionDevboxes: obj{
			"type":          "object",
			"description":   "Devboxes authama keeps as SSH hosts, by alias. `ssh vm sync` adds a declared devbox that is not a host yet and adopts its loose key (a copy) when the profile lacks it. A devbox without vm is planned: it only keeps its key (doctor reports it as reserved) until the VM exists.",
			"propertyNames": obj{"pattern": "^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$"},
			"additionalProperties": obj{
				"type": "object", "additionalProperties": false, "required": []string{"profile"},
				"if":   obj{"required": []string{"vm"}},
				"then": obj{"required": []string{"hypervisor", "vm", "user"}},
				"else": obj{"required": []string{"key"}},
				"properties": obj{
					"profile":        obj{"type": "string", "minLength": 1, "description": "The profile the host belongs to (created when missing)."},
					"hypervisor":     obj{"type": "string", "minLength": 1, "description": "A built-in hypervisor (" + strings.Join(manifest.Hypervisors[:5], ", ") + ") or one from the hypervisors section."},
					"vm":             obj{"type": "string", "minLength": 1, "description": "The VM's name as the hypervisor lists it. Leave it out for a planned devbox that only keeps its key."},
					"user":           obj{"type": "string", "pattern": "^[A-Za-z0-9_][A-Za-z0-9._-]{0,31}$", "description": "The guest user."},
					"key":            obj{"type": "string", "pattern": "^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$", "description": "The key name the host uses; a loose ~/.ssh/<key> pair is adopted (copied) when the profile has no key of that name."},
					"docker_context": obj{"type": "boolean", "default": true, "description": "Keep a Docker context ssh://<alias>."},
					"description":    obj{"type": "string"},
				},
			},
		},
	}
}

// LoadVMConfig reads the structured sections of the authama.json at path and
// registers its custom hypervisors. A missing file is an empty config.
func LoadVMConfig(path string) (VMConfig, error) {
	f, err := LoadFile(path)
	if err != nil {
		return VMConfig{}, err
	}
	if f == nil {
		hypervisor.Register(nil)
		return VMConfig{}, nil
	}
	hypervisor.Register(f.VM.Hypervisors)
	return f.VM, nil
}
