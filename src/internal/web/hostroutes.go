package web

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/editor"
)

// routeKeys are the ssh options the host forms give a field of their own,
// so the options box leaves them out.
var routeKeys = []string{"ProxyJump", "ConnectTimeout"}

// RouteView is a host's routes as its edit form shows them: the extra Host
// names, the proxy, the options and comments, and its route variants.
type RouteView struct {
	Aliases                   string
	Proxy                     string // cloudflared, none, command or ""
	ProxyCommand              string
	ProxyJump, ConnectTimeout string
	Options                   string // KEY=VALUE, one a line
	Section, Comment          string
	Variants                  []VariantView
	// Services are currently attached; ServiceOptions is the checkbox list
	// (built-in kinds plus any attached name that is not built in).
	Services       []string
	ServiceOptions []ServiceOption
}

// ServiceOption is one checkbox on a host or variant form.
type ServiceOption struct {
	Name    string
	Checked bool
}

// routeFormData is what the route_fields partial is given.
type routeFormData struct {
	P             string
	R             RouteView
	Edit, Variant bool
}

// VariantView is one route variant: the Host it renders and what it sets.
type VariantView struct {
	Name, Alias, Hostname, Port, Proxy, ConnectTimeout, Options, Section, Comment string
	// Route is what its edit form starts with.
	Route RouteView
}

func proxyLabel(p *manifest.ProxySpec) (kind, command string) {
	if p == nil {
		return "", ""
	}
	if p.Kind == manifest.ProxyCommandKind {
		return p.Kind, manifest.ShellJoin(p.Command)
	}
	return p.Kind, ""
}

func optionLines(o manifest.OrderedOptions) string {
	var b strings.Builder
	for _, k := range o.Keys() {
		if slices.Contains(routeKeys, k) {
			continue
		}
		b.WriteString(k + "=" + o.Get(k) + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func serviceOptions(cur []string) []ServiceOption {
	seen := map[string]bool{}
	var out []ServiceOption
	for _, n := range manifest.BuiltinServiceKinds() {
		out = append(out, ServiceOption{Name: n, Checked: slices.Contains(cur, n)})
		seen[n] = true
	}
	for _, n := range cur {
		if n != "" && !seen[n] {
			out = append(out, ServiceOption{Name: n, Checked: true})
			seen[n] = true
		}
	}
	return out
}

func routeView(h manifest.Host) RouteView {
	v := RouteView{Aliases: strings.Join(h.Aliases, ", "), ProxyJump: h.RawOptions.Get("ProxyJump"),
		ConnectTimeout: h.RawOptions.Get("ConnectTimeout"), Options: optionLines(h.RawOptions),
		Section: h.Section, Comment: h.Comment, Services: append([]string(nil), h.Services...),
		ServiceOptions: serviceOptions(h.Services)}
	v.Proxy, v.ProxyCommand = proxyLabel(h.Proxy)
	for _, x := range h.Variants {
		vv := VariantView{Name: x.Name, Alias: manifest.VariantAlias(x.Name, h.Alias), Hostname: x.Hostname,
			ConnectTimeout: x.RawOptions.Get("ConnectTimeout"), Options: optionLines(x.RawOptions),
			Section: x.Section, Comment: x.Comment}
		if x.Port != 0 {
			vv.Port = strconv.Itoa(x.Port)
		}
		vv.Route = routeView(manifest.Host{Proxy: x.Proxy, RawOptions: x.RawOptions, Section: x.Section, Comment: x.Comment, Services: x.Services})
		vv.Proxy, _ = proxyLabel(x.Proxy)
		if k, c := proxyLabel(x.Proxy); c != "" {
			vv.Proxy = k + ": " + c
		}
		v.Variants = append(v.Variants, vv)
	}
	return v
}

// nameList reads a comma- or space-separated list of Host names.
func nameList(f url.Values, field string) ([]string, error) {
	var out []string
	for _, x := range strings.FieldsFunc(f.Get(field), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if !nameRe.MatchString(x) {
			return nil, badForm("%s %q is not a name authama accepts: letters, digits, '.', '_' and '-'", field, x)
		}
		if !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out, nil
}

// optionList reads the options box: KEY=VALUE (or KEY VALUE), one a line,
// each an ssh_config option OpenSSH knows that a host may set.
func optionList(f url.Values) ([][2]string, error) {
	v, err := lines(f, "options")
	if err != nil {
		return nil, err
	}
	var out [][2]string
	for _, l := range strings.Split(v, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		kv, err := editor.ParseOption(l)
		if err != nil {
			return nil, badForm("%s", err.Error())
		}
		kv[0] = manifest.CanonicalOption(kv[0])
		if strings.EqualFold(kv[0], "ProxyCommand") {
			return nil, badForm("a ProxyCommand is set with the proxy fields, not as an option")
		}
		if err := manifest.CheckHostOption(kv[0], kv[1]); err != nil {
			return nil, badForm("%s", err.Error())
		}
		out = append(out, kv)
	}
	return out, nil
}

// proxyKind reads the proxy select: cloudflared, none, off, or "" to leave it.
func proxyKind(f url.Values) (*string, error) {
	v := strings.TrimSpace(f.Get("proxy"))
	switch {
	case v == "":
		return nil, nil
	case v == editor.ProxyOff, v != manifest.ProxyCommandKind && slices.Contains(manifest.ProxyKinds, v):
		return &v, nil
	}
	return nil, badForm("proxy must be cloudflared, none (ProxyCommand none) or off (remove it)")
}

func timeoutOf(f url.Values) (int, error) {
	v := strings.TrimSpace(f.Get("connect_timeout"))
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 3600 {
		return 0, badForm("connect timeout is a number of seconds, 0 to 3600")
	}
	return n, nil
}

// routeInput reads the route fields of a host or variant form. With cur, an
// edit form shown filled in, only what differs from cur becomes a flag, and an
// option taken out of the box is unset; without it every field given is one.
func routeInput(f url.Values, cur *manifest.Host) (change.RouteInput, error) {
	var in change.RouteInput
	var err error
	if in.Proxy, err = proxyKind(f); err != nil {
		return in, err
	}
	str := func(field, was string) (*string, error) {
		if _, ok := f[field]; !ok {
			return nil, nil
		}
		v, err := plain(f, field)
		if err != nil || v == was || (cur == nil && v == "") {
			return nil, err
		}
		return &v, nil
	}
	curOpts := manifest.OrderedOptions{}
	var curHost manifest.Host
	if cur != nil {
		curHost, curOpts = *cur, cur.RawOptions
	}
	_, curCmd := proxyLabel(curHost.Proxy)
	if in.ProxyCommand, err = str("proxy_command", curCmd); err != nil {
		return in, err
	}
	if in.ProxyCommand != nil && *in.ProxyCommand != "" {
		if _, err := manifest.ProxyFromCommand(*in.ProxyCommand); err != nil {
			return in, badForm("%s", err.Error())
		}
	}
	if in.ProxyJump, err = str("proxy_jump", curOpts.Get("ProxyJump")); err != nil {
		return in, err
	}
	if in.ProxyJump != nil && *in.ProxyJump != "" {
		if err := manifest.CheckHostOption("ProxyJump", *in.ProxyJump); err != nil {
			return in, badForm("%s", err.Error())
		}
	}
	if _, ok := f["connect_timeout"]; ok {
		n, err := timeoutOf(f)
		if err != nil {
			return in, err
		}
		was, _ := strconv.Atoi(curOpts.Get("ConnectTimeout"))
		if n != was {
			in.ConnectTimeout = &n
		}
	}
	if _, ok := f["options"]; ok {
		opts, err := optionList(f)
		if err != nil {
			return in, err
		}
		given := map[string]bool{}
		for _, kv := range opts {
			given[strings.ToLower(kv[0])] = true
			if curOpts.Get(kv[0]) != kv[1] {
				in.Options = append(in.Options, kv)
			}
		}
		for _, k := range curOpts.Keys() {
			if cur != nil && !slices.Contains(routeKeys, k) && !given[strings.ToLower(k)] {
				in.Unset = append(in.Unset, k)
			}
		}
	}
	if in.Section, err = str("section", curHost.Section); err != nil {
		return in, err
	}
	if in.Comment, err = str("comment", curHost.Comment); err != nil {
		return in, err
	}
	if _, ok := f["service_set"]; ok {
		var selected []string
		for _, n := range f["service"] {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if !nameRe.MatchString(n) {
				return in, badForm("service %q is not a name authama accepts: letters, digits, '.', '_' and '-'", n)
			}
			if !slices.Contains(selected, n) {
				selected = append(selected, n)
			}
		}
		if extra := strings.TrimSpace(f.Get("service_extra")); extra != "" {
			for _, n := range strings.FieldsFunc(extra, func(r rune) bool { return r == ',' || r == ' ' || r == '	' }) {
				if !nameRe.MatchString(n) {
					return in, badForm("service %q is not a name authama accepts: letters, digits, '.', '_' and '-'", n)
				}
				if !slices.Contains(selected, n) {
					selected = append(selected, n)
				}
			}
		}
		if cur == nil {
			in.Services = selected
		} else {
			for _, n := range selected {
				if !slices.Contains(curHost.Services, n) {
					in.Services = append(in.Services, n)
				}
			}
			for _, n := range curHost.Services {
				if !slices.Contains(selected, n) {
					in.RemoveServices = append(in.RemoveServices, n)
				}
			}
		}
	}
	return in, nil
}

// hostRoutes reads a host form's extra names and routes. On an edit the
// names box holds the whole new list: names left out are removed.
func hostRoutes(f url.Values, h home, profile, alias string, edit bool) (add, remove []string, in change.RouteInput, err error) {
	var cur *manifest.Host
	if edit {
		m, err := manifest.Load(h.p.Manifest())
		if err != nil {
			return nil, nil, in, err
		}
		for _, x := range m.Profiles[profile].Hosts {
			if x.Alias == alias {
				cur = &x
				break
			}
		}
		if cur == nil {
			return nil, nil, in, badForm("%s has no host %s", profile, alias)
		}
	}
	if _, ok := f["aliases"]; ok {
		names, err := nameList(f, "aliases")
		if err != nil {
			return nil, nil, in, err
		}
		var was []string
		if cur != nil {
			was = cur.Aliases
		}
		for _, x := range names {
			if !slices.Contains(was, x) {
				add = append(add, x)
			}
		}
		for _, x := range was {
			if !slices.Contains(names, x) {
				remove = append(remove, x)
			}
		}
	}
	in, err = routeInput(f, cur)
	return add, remove, in, err
}

// variantInput reads the route variant form. A variant the host has is
// edited as the host is: its form starts filled in, only what differs is a
// flag, and an option taken out is unset. A new one takes what is given.
func variantInput(f url.Values, h home) (change.VariantInput, error) {
	var in change.VariantInput
	var cur *manifest.Variant
	if m, err := manifest.Load(h.p.Manifest()); err == nil {
		for _, x := range m.Profiles[strings.TrimSpace(f.Get("profile"))].Hosts {
			if x.Alias == strings.TrimSpace(f.Get("alias")) {
				if v, ok := x.Variant(strings.TrimSpace(f.Get("name"))); ok {
					cur = &v
				}
			}
		}
	}
	hostname, err := plain(f, "hostname")
	if err != nil {
		return in, err
	}
	port, err := portOf(f)
	if err != nil {
		return in, err
	}
	if cur == nil {
		if hostname != "" {
			in.Hostname = &hostname
		}
		in.Port = port
		in.RouteInput, err = routeInput(f, nil)
		return in, err
	}
	if _, ok := f["hostname"]; ok && hostname != cur.Hostname {
		in.Hostname = &hostname
	}
	if _, ok := f["port"]; ok {
		was := cur.Port
		switch {
		case port != nil && *port != was:
			in.Port = port
		case port == nil && was != 0:
			zero := 0 // the host's port again
			in.Port = &zero
		}
	}
	in.RouteInput, err = routeInput(f, &manifest.Host{Proxy: cur.Proxy, RawOptions: cur.RawOptions,
		Section: cur.Section, Comment: cur.Comment, Services: cur.Services})
	return in, err
}
