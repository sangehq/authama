package editor

import (
	"fmt"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/log"
)

// Routes holds the host edits that create-or-update: upserting a host, its
// route variants, and taking hand edits back from the managed files. It is the
// Editor's, kept apart so neither type carries every verb.
type Routes struct{ e *Editor }

// RoutesOf is e's create-or-update edits.
func RoutesOf(e *Editor) Routes { return Routes{e: e} }

// ProxyOff is the --proxy value that removes a host's proxy, so its block has
// no ProxyCommand line at all. "none" is a proxy of its own: it renders
// `ProxyCommand none`, which connects directly even under a broader block
// that sets a proxy.
const ProxyOff = "off"

// RouteFields are the settings a host and a route variant share: how ssh
// reaches the server, extra options, and the comments rendered around the
// block. nil and empty leave each as it is.
type RouteFields struct {
	// Proxy is a proxy kind: cloudflared, none, or off ("" too) to remove it.
	Proxy *string
	// ProxyCommand is a command line of the user's own, split into an argv
	// and quoted again on render; "" removes it. Its value may also be none
	// or the Cloudflare Access command, which become those kinds.
	ProxyCommand *string
	// SetOptions are ssh options to set, in order; UnsetOptions keywords to drop.
	SetOptions   [][2]string
	UnsetOptions []string
	Section      *string
	Comment      *string
}

// VariantFields are a route variant's settings on top of RouteFields.
type VariantFields struct {
	Hostname *string
	Port     *int
	RouteFields
}

func (f RouteFields) empty() bool {
	return f.Proxy == nil && f.ProxyCommand == nil && len(f.SetOptions) == 0 && len(f.UnsetOptions) == 0 &&
		f.Section == nil && f.Comment == nil
}

// resolveProxy applies --proxy / --proxy-command to the current proxy.
func resolveProxy(cur *manifest.ProxySpec, f RouteFields) (*manifest.ProxySpec, error) {
	if f.Proxy != nil && f.ProxyCommand != nil && *f.Proxy != "" && *f.ProxyCommand != "" {
		return nil, fmt.Errorf("--proxy and --proxy-command both set the ProxyCommand: give one")
	}
	if f.ProxyCommand != nil {
		if strings.TrimSpace(*f.ProxyCommand) == "" {
			return nil, nil
		}
		return manifest.ProxyFromCommand(*f.ProxyCommand)
	}
	if f.Proxy == nil {
		return cur, nil
	}
	switch k := strings.TrimSpace(*f.Proxy); k {
	case "", ProxyOff:
		return nil, nil
	case manifest.ProxyCommandKind:
		return nil, fmt.Errorf("a command proxy is given with --proxy-command '<command>'")
	default:
		for _, known := range manifest.ProxyKinds {
			if k == known {
				return &manifest.ProxySpec{Kind: k}, nil
			}
		}
		return nil, fmt.Errorf("--proxy %q is not a proxy authama renders (cloudflared, none, or off to remove it)", k)
	}
}

// applyOptions sets and unsets ssh options, each checked against the
// keywords OpenSSH knows and written in its spelling.
func applyOptions(o manifest.OrderedOptions, f RouteFields) (manifest.OrderedOptions, error) {
	for _, k := range f.UnsetOptions {
		o = o.Without(k)
	}
	for _, kv := range f.SetOptions {
		k, v := manifest.CanonicalOption(strings.TrimSpace(kv[0])), strings.TrimSpace(kv[1])
		if v == "" {
			o = o.Without(k)
			continue
		}
		if err := manifest.CheckHostOption(k, v); err != nil {
			return o, err
		}
		o.Set(k, v)
	}
	return o, nil
}

// ParseOption reads a KEY=VALUE (or "KEY VALUE") option argument.
func ParseOption(s string) ([2]string, error) {
	s = strings.TrimSpace(s)
	i := strings.IndexAny(s, "= \t")
	if i <= 0 {
		return [2]string{}, fmt.Errorf("an option is KEY=VALUE, e.g. ServerAliveInterval=30 (got %q)", s)
	}
	return [2]string{s[:i], strings.TrimSpace(s[i+1:])}, nil
}

// applyRoute applies RouteFields to a host.
func applyRoute(h *manifest.Host, f RouteFields) error {
	p, err := resolveProxy(h.Proxy, f)
	if err != nil {
		return err
	}
	h.Proxy = p
	if h.RawOptions, err = applyOptions(h.RawOptions, f); err != nil {
		return err
	}
	if f.Section != nil {
		h.Section = strings.TrimSpace(*f.Section)
	}
	if f.Comment != nil {
		h.Comment = strings.TrimSpace(*f.Comment)
	}
	return nil
}

// applyAliases adds and removes a host's extra names.
func applyAliases(h *manifest.Host, add, remove []string) error {
	for _, a := range remove {
		kept := h.Aliases[:0:0]
		found := false
		for _, x := range h.Aliases {
			if x == a {
				found = true
				continue
			}
			kept = append(kept, x)
		}
		if !found {
			return fmt.Errorf("%s has no alias %q (it has: %s)", h.Alias, a, strings.Join(h.Aliases, ", "))
		}
		h.Aliases = kept
	}
	for _, a := range add {
		if a == h.Alias || containsStr(h.Aliases, a) {
			continue
		}
		h.Aliases = append(h.Aliases, a)
	}
	return nil
}

// UpsertHost adds the host, or updates it when the profile has it already.
// It reports whether it was added.
func (r Routes) UpsertHost(profile, alias string, f HostFields) (added bool, err error) {
	e := r.e
	m, err := e.load()
	if err != nil {
		return false, err
	}
	idx, _, err := findHost(m, profile, alias)
	if err != nil {
		if deref(f.Hostname) == "" || deref(f.User) == "" {
			return false, fmt.Errorf("%s is not in %s yet: a new host needs --hostname and --user", alias, profile)
		}
		return true, e.AddHost(profile, alias, f)
	}
	// An update changes what was asked for: an empty --hostname or --user,
	// or a --port left at its default, is not a request to change them, and
	// tags are added to the host's own.
	if f.Hostname != nil && *f.Hostname == "" {
		f.Hostname = nil
	}
	if f.User != nil && *f.User == "" {
		f.User = nil
	}
	if !f.PortSet {
		f.Port = nil
	}
	if len(f.Tags) > 0 {
		tags := append(append([]string{}, m.Profiles[profile].Hosts[idx].Tags...), f.Tags...)
		f.SetTags = &tags
	}
	f.Tags = nil
	return false, e.EditHost(profile, alias, f)
}

// SetVariant creates or updates a host's route variant. It reports whether
// it was created.
func (r Routes) SetVariant(profile, alias, name string, f VariantFields) (created bool, err error) {
	e := r.e
	m, err := e.load()
	if err != nil {
		return false, err
	}
	idx, host, err := findHost(m, profile, alias)
	if err != nil {
		return false, err
	}
	v, ok := host.Variant(name)
	if !ok {
		v = manifest.Variant{Name: name}
	}
	if f.Hostname != nil {
		v.Hostname = strings.TrimSpace(*f.Hostname)
		if v.Hostname == host.Hostname {
			v.Hostname = ""
		}
	}
	if f.Port != nil {
		v.Port = *f.Port
		if v.Port == host.Port {
			v.Port = 0
		}
	}
	if v.Proxy, err = resolveProxy(v.Proxy, f.RouteFields); err != nil {
		return false, err
	}
	if v.RawOptions, err = applyOptions(v.RawOptions, f.RouteFields); err != nil {
		return false, err
	}
	if f.Section != nil {
		v.Section = strings.TrimSpace(*f.Section)
	}
	if f.Comment != nil {
		v.Comment = strings.TrimSpace(*f.Comment)
	}
	if ok {
		for i := range host.Variants {
			if host.Variants[i].Name == name {
				host.Variants[i] = v
			}
		}
	} else {
		host.Variants = append(host.Variants, v)
	}
	p := m.Profiles[profile]
	p.Hosts[idx] = host
	m.SetProfile(profile, p)
	if err := e.save(m); err != nil {
		return false, err
	}
	log.Audit(e.p.AuditLog(), "host.variant.set", log.Field{Key: "profile", Value: profile},
		log.Field{Key: "alias", Value: alias}, log.Field{Key: "variant", Value: name})
	return !ok, nil
}

// RemoveVariant removes a host's route variant.
func (r Routes) RemoveVariant(profile, alias, name string) error {
	e := r.e
	m, err := e.load()
	if err != nil {
		return err
	}
	idx, host, err := findHost(m, profile, alias)
	if err != nil {
		return err
	}
	if _, ok := host.Variant(name); !ok {
		return fmt.Errorf("%s has no variant %q", alias, name)
	}
	kept := host.Variants[:0:0]
	for _, v := range host.Variants {
		if v.Name != name {
			kept = append(kept, v)
		}
	}
	host.Variants = kept
	p := m.Profiles[profile]
	p.Hosts[idx] = host
	m.SetProfile(profile, p)
	if err := e.save(m); err != nil {
		return err
	}
	log.Audit(e.p.AuditLog(), "host.variant.remove", log.Field{Key: "profile", Value: profile},
		log.Field{Key: "alias", Value: alias}, log.Field{Key: "variant", Value: name})
	return nil
}
