package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Variant is a second route to the same server: the Cloudflare Access route
// is the host itself, and a WARP or VPN route to its private address is a
// variant named "warp", rendered as its own Host block called warp-<alias>.
// The variant shares the host's user and key, so rotating the key or
// changing the user moves both routes at once; it overrides only how ssh
// gets there - the address, the port, the proxy and any option it sets.
type Variant struct {
	// Name is the route's name and the prefix of its alias: "warp" renders
	// warp-<alias>.
	Name string `json:"name"`
	// Hostname and Port replace the host's when set.
	Hostname string `json:"hostname,omitempty"`
	Port     int    `json:"port,omitempty"`
	// Proxy replaces the host's proxy when set ({"kind":"none"} connects
	// directly); unset keeps the host's.
	Proxy *ProxySpec `json:"proxy,omitempty"`
	// RawOptions are added to the host's own, a key both set taking this value.
	RawOptions OrderedOptions `json:"raw_options"`
	Section    string         `json:"section,omitempty"`
	Comment    string         `json:"comment,omitempty"`
}

// MarshalJSON leaves out empty raw_options, so a variant reads as what it changes.
func (v Variant) MarshalJSON() ([]byte, error) {
	type wire struct {
		Name       string          `json:"name"`
		Hostname   string          `json:"hostname,omitempty"`
		Port       int             `json:"port,omitempty"`
		Proxy      *ProxySpec      `json:"proxy,omitempty"`
		RawOptions *OrderedOptions `json:"raw_options,omitempty"`
		Section    string          `json:"section,omitempty"`
		Comment    string          `json:"comment,omitempty"`
	}
	w := wire{Name: v.Name, Hostname: v.Hostname, Port: v.Port, Proxy: v.Proxy, Section: v.Section, Comment: v.Comment}
	if v.RawOptions.Len() > 0 {
		w.RawOptions = &v.RawOptions
	}
	return json.Marshal(w)
}

// UnmarshalJSON is strict, like a host: an unknown key is a typo.
func (v *Variant) UnmarshalJSON(b []byte) error {
	type alias Variant
	var aux alias
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return err
	}
	*v = Variant(aux)
	return nil
}

var variantName = regexp.MustCompile(`^[a-z0-9][a-z0-9_]*$`)

// VariantAlias is the alias a variant renders: <name>-<alias>.
func VariantAlias(variant, alias string) string { return variant + "-" + alias }

// Variant returns the host's variant of that name.
func (h Host) Variant(name string) (Variant, bool) {
	for _, v := range h.Variants {
		if v.Name == name {
			return v, true
		}
	}
	return Variant{}, false
}

// Names are every Host name the host renders: its alias, its extra
// aliases, and each variant's alias.
func (h Host) Names() []string {
	out := append([]string{h.Alias}, h.Aliases...)
	for _, v := range h.Variants {
		out = append(out, VariantAlias(v.Name, h.Alias))
	}
	return out
}

// VariantHost is the host a variant renders as: the base host with the
// variant's route. Its key is the base host's resolved key, named
// explicitly, because a per-service key is otherwise derived from the
// alias and the variant's alias differs.
func (m *Manifest) VariantHost(profile string, base Host, v Variant) (Host, error) {
	kname, err := m.ResolvedKeyName(profile, base)
	if err != nil {
		return Host{}, err
	}
	h := Host{
		Alias: VariantAlias(v.Name, base.Alias), Hostname: base.Hostname, User: base.User, Port: base.Port,
		KeyName: &kname, Tags: base.Tags, Proxy: base.Proxy, Section: v.Section, Comment: v.Comment,
		RequiresVPN: base.RequiresVPN, VPNName: base.VPNName, VPNURL: base.VPNURL,
	}
	if v.Hostname != "" {
		h.Hostname = v.Hostname
	}
	if v.Port != 0 {
		h.Port = v.Port
	}
	if v.Proxy != nil {
		h.Proxy = v.Proxy
	}
	var pairs [][2]string
	for _, k := range base.RawOptions.Keys() {
		pairs = append(pairs, [2]string{k, base.RawOptions.Get(k)})
	}
	h.RawOptions = NewOrderedOptions(pairs)
	for _, k := range v.RawOptions.Keys() {
		h.RawOptions.Set(k, v.RawOptions.Get(k))
	}
	if v.Proxy != nil && hasRawKey(h.RawOptions, "ProxyJump") && !hasRawKey(v.RawOptions, "ProxyJump") {
		// The variant's own proxy replaces the route; a jump inherited from
		// the base would otherwise win over it.
		h.RawOptions = withoutKey(h.RawOptions, "ProxyJump")
	}
	return h, nil
}

func withoutKey(o OrderedOptions, key string) OrderedOptions {
	var pairs [][2]string
	for _, k := range o.Keys() {
		if !strings.EqualFold(k, key) {
			pairs = append(pairs, [2]string{k, o.Get(k)})
		}
	}
	return NewOrderedOptions(pairs)
}

// Without returns the options without key (any case).
func (o OrderedOptions) Without(key string) OrderedOptions { return withoutKey(o, key) }

// checkHostName validates a name ssh is given as a Host pattern and a destination.
func checkHostName(field, value string) error {
	if err := safeSegment(field, value); err != nil {
		return err
	}
	if err := rejectConfigSyntax(field, value); err != nil {
		return err
	}
	return rejectSSHRefused(field, value, sshHostRefused)
}

// checkComment refuses a comment or section that would end its line.
func checkComment(field, value string) error {
	return rejectControl(field, value)
}

// checkRoutes validates the fields this file adds to a host: its extra
// aliases, comment, section and variants.
func checkRoutes(profile string, h Host) error {
	where := profile + "/" + h.Alias
	seen := map[string]bool{strings.ToLower(h.Alias): true}
	for _, a := range h.Aliases {
		if err := checkHostName("aliases", a); err != nil {
			return err
		}
		if seen[strings.ToLower(a)] {
			return fmt.Errorf("%s: alias %q is listed twice", where, a)
		}
		seen[strings.ToLower(a)] = true
	}
	if err := checkComment("comment", h.Comment); err != nil {
		return err
	}
	if err := checkComment("section", h.Section); err != nil {
		return err
	}
	if len(h.Variants) > 0 && h.VM != nil {
		return fmt.Errorf("%s: a VM host cannot have route variants (authama asks the hypervisor for its address)", where)
	}
	names := map[string]bool{}
	for _, v := range h.Variants {
		vw := where + " variant " + v.Name
		if !variantName.MatchString(v.Name) {
			return fmt.Errorf("%s: variant name %q must be lower-case letters, digits and '_' "+
				"(it prefixes the alias: warp-<alias>)", where, v.Name)
		}
		if names[v.Name] {
			return fmt.Errorf("%s: variant %q is declared twice", where, v.Name)
		}
		names[v.Name] = true
		if err := checkHostName("variant alias", VariantAlias(v.Name, h.Alias)); err != nil {
			return err
		}
		if v.Hostname != "" {
			if err := safeValue("variant hostname", v.Hostname); err != nil {
				return err
			}
			if err := rejectSSHRefused("variant hostname", v.Hostname, sshHostRefused); err != nil {
				return err
			}
		}
		if v.Port < 0 || v.Port > 65535 {
			return fmt.Errorf("%s: port %d is out of range", vw, v.Port)
		}
		if err := checkOptions("variant raw_options", v.RawOptions); err != nil {
			return err
		}
		if err := checkProxySpec(vw, v.Proxy); err != nil {
			return err
		}
		if v.Proxy != nil && hasRawKey(v.RawOptions, "ProxyJump") {
			return fmt.Errorf("%s: set a proxy or a ProxyJump, not both", vw)
		}
		if err := checkComment("variant section", v.Section); err != nil {
			return err
		}
		if err := checkComment("variant comment", v.Comment); err != nil {
			return err
		}
	}
	return nil
}
