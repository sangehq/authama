package editor

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/renderer"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/util/log"
)

// Adopted is one hand-edited block taken into the manifest.
type Adopted struct {
	File, Profile, Alias string
	// Action is "added host", "updated host", "added variant warp of web" ...
	Action string
}

func (a Adopted) String() string {
	return fmt.Sprintf("%s/%s: %s (from %s)", a.Profile, a.Alias, a.Action, a.File)
}

// AdoptResult is what keep-edits did, or would do.
type AdoptResult struct {
	Adopted []Adopted
	// Left are the edits it could not take in, each with the reason; a render
	// still refuses over them.
	Left []string
}

// Format is the report keep-edits prints.
func (r AdoptResult) Format(dryRun bool) string {
	var b strings.Builder
	verb := "adopted"
	if dryRun {
		verb = "would adopt"
	}
	if len(r.Adopted) == 0 && len(r.Left) == 0 {
		return "no hand edits in the managed config: nothing to adopt"
	}
	for _, a := range r.Adopted {
		fmt.Fprintf(&b, "%s %s\n", verb, a)
	}
	for _, l := range r.Left {
		fmt.Fprintf(&b, "left   %s\n", l)
	}
	return strings.TrimRight(b.String(), "\n")
}

// keptOut are the lines authama writes into every block itself, so a hand
// block carrying them adds nothing to keep.
func keptOut(m *manifest.Manifest, key, value, agent string) bool {
	switch strings.ToLower(key) {
	case "hostname", "user", "port", "identityfile", "proxycommand":
		return true // fields of the host, read separately
	case "identitiesonly":
		return strings.EqualFold(value, "yes")
	case "userknownhostsfile":
		return value == m.KnownHostsFile()
	case "identityagent":
		return strings.Trim(value, `"`) == agent
	}
	return false
}

// hostFromBlock reads a hand-written block into a host of profile: its
// address, extra names, proxy, options, heading and comment. keyName is the
// key its IdentityFile names, "" for the one the alias derives.
func hostFromBlock(m *manifest.Manifest, profile string, b renderer.Block) (manifest.Host, error) {
	h := manifest.Host{Alias: b.Alias(), Aliases: b.Patterns[1:], Section: b.Section, Comment: b.Comment, Port: 22}
	var ok bool
	if h.Hostname, ok = b.Get("HostName"); !ok {
		return h, fmt.Errorf("it sets no HostName")
	}
	if h.User, ok = b.Get("User"); !ok {
		return h, fmt.Errorf("it sets no User")
	}
	if p, ok := b.Get("Port"); ok {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return h, fmt.Errorf("its Port %q is not a port number", p)
		}
		h.Port = n
	}
	if pc, ok := b.Get("ProxyCommand"); ok {
		if renderer.ProxyBinary(pc) != "" && strings.Contains(pc, " vm proxy ") {
			return h, fmt.Errorf("its ProxyCommand is a VM host's: add VM hosts with `authama ssh vm add`")
		}
		p, err := manifest.ProxyFromCommand(pc)
		if err != nil {
			return h, fmt.Errorf("ProxyCommand: %w", err)
		}
		h.Proxy = p
	}
	agent := ""
	if m.Profiles[profile].InOnePassword() {
		agent = m.Defaults.OnePasswordAgentSocket()
	}
	var pairs [][2]string
	seen := map[string]bool{}
	for _, o := range b.Options {
		k := strings.ToLower(o.Key)
		if keptOut(m, o.Key, o.Value, agent) || seen[k] {
			continue // ssh reads the first value of a keyword; so does authama
		}
		if k == "userknownhostsfile" {
			return h, fmt.Errorf("UserKnownHostsFile %s: authama keeps every pin in %s", o.Value, m.KnownHostsFile())
		}
		if !manifest.IsSSHOption(o.Key) {
			return h, fmt.Errorf("%s is not an ssh_config keyword ssh knows", o.Key)
		}
		if manifest.IsDangerousOption(o.Key) {
			return h, fmt.Errorf("%s can run a command or load config, and is not taken into the manifest", o.Key)
		}
		if err := manifest.CheckOptionValue(o.Key, o.Value); err != nil {
			return h, err
		}
		seen[k] = true
		pairs = append(pairs, [2]string{manifest.CanonicalOption(o.Key), o.Value})
	}
	h.RawOptions = manifest.NewOrderedOptions(pairs)
	return h, nil
}

// keyFor works out the key_name a block's IdentityFile means for alias in
// profile: nil for the key the alias derives, the file name for another key in
// the profile's directory, and an error for a key anywhere else.
func keyFor(m *manifest.Manifest, profile, alias string, b renderer.Block) (*string, error) {
	file, ok := b.Get("IdentityFile")
	if !ok {
		return nil, fmt.Errorf("it sets no IdentityFile")
	}
	file = strings.TrimSuffix(strings.Trim(file, `"`), ".pub")
	derived, err := m.ResolvedKeyName(profile, manifest.Host{Alias: alias})
	if err == nil && file == strings.TrimSuffix(m.IdentityFile(profile, derived), ".pub") {
		return nil, nil
	}
	dir, name := path.Split(file)
	if strings.TrimSuffix(dir, "/") == "~/.ssh/"+m.ProfileRel(profile) {
		return &name, nil
	}
	return nil, fmt.Errorf("its IdentityFile %s is not in the profile's key directory ~/.ssh/%s "+
		"(bring the key in first with `authama ssh key adopt`)", file, m.ProfileRel(profile))
}

// profileFor is the profile a block belongs to: the banner above it, or the
// one profile the file holds.
func profileFor(m *manifest.Manifest, file string, b renderer.Block) (string, error) {
	if b.Profile != "" {
		if _, ok := m.Profiles[b.Profile]; ok {
			return b.Profile, nil
		}
		return "", fmt.Errorf("it is under the banner of profile %q, which the manifest does not have", b.Profile)
	}
	var in []string
	for _, p := range m.ProfileNames() {
		c := m.Profiles[p].Category
		if (file == renderer.RootConfig && c == "") || (c != "" && file == manifest.CategoryConfigRelIn(manifest.KeyRoot(m), c)) {
			in = append(in, p)
		}
	}
	if len(in) == 1 {
		return in[0], nil
	}
	return "", fmt.Errorf("it is under no profile banner: move it under its profile's `# --- profile:` line")
}

// findAlias finds the host that renders name: as its alias, an extra alias,
// or a variant. The variant's name is "" for the host itself.
func findAlias(m *manifest.Manifest, profile, name string) (idx int, variant string, ok bool) {
	for i, h := range m.Profiles[profile].Hosts {
		if h.Alias == name || containsStr(h.Aliases, name) {
			return i, "", true
		}
		for _, v := range h.Variants {
			if manifest.VariantAlias(v.Name, h.Alias) == name {
				return i, v.Name, true
			}
		}
	}
	return 0, "", false
}

// asVariant reads a block as a route variant of a host already in the
// profile: its alias is <name>-<alias>, and it has the host's user and key.
func asVariant(m *manifest.Manifest, profile string, b renderer.Block) (idx int, name string, ok bool) {
	if len(b.Patterns) != 1 {
		return 0, "", false
	}
	user, _ := b.Get("User")
	file, _ := b.Get("IdentityFile")
	for i, h := range m.Profiles[profile].Hosts {
		prefix, found := strings.CutSuffix(b.Alias(), "-"+h.Alias)
		if !found || prefix == "" || strings.Contains(prefix, "-") || h.VM != nil {
			continue
		}
		kn, err := m.ResolvedKeyName(profile, h)
		if err != nil || user != h.User || strings.TrimSuffix(file, ".pub") != strings.TrimSuffix(m.IdentityFile(profile, kn), ".pub") {
			continue
		}
		return i, prefix, true
	}
	return 0, "", false
}

// variantFrom is the variant that, on top of base, renders as h.
func variantFrom(name string, base, h manifest.Host) manifest.Variant {
	v := manifest.Variant{Name: name, Section: h.Section, Comment: h.Comment}
	if h.Hostname != base.Hostname {
		v.Hostname = h.Hostname
	}
	if h.Port != base.Port {
		v.Port = h.Port
	}
	switch {
	case h.Proxy != nil:
		v.Proxy = h.Proxy
	case base.Proxy != nil:
		// The block has no ProxyCommand where its host has one: it connects
		// directly, which a variant says with none.
		v.Proxy = &manifest.ProxySpec{Kind: manifest.ProxyNone}
	}
	var pairs [][2]string
	for _, k := range h.RawOptions.Keys() {
		if base.RawOptions.Get(k) != h.RawOptions.Get(k) {
			pairs = append(pairs, [2]string{k, h.RawOptions.Get(k)})
		}
	}
	v.RawOptions = manifest.NewOrderedOptions(pairs)
	return v
}

// AdoptEdits takes what was typed into the managed config by hand into the
// manifest, so the next render keeps it instead of refusing over it:
//
//   - a block for a host the profile has updates that host (address, names,
//     proxy, options, heading, comment); for a variant, that variant;
//   - a block named <name>-<alias> with the user and key of a host <alias>
//     becomes that host's route variant <name>;
//   - any other block becomes a new host of the profile whose banner it is
//     under, with the key its IdentityFile names in the profile's directory.
//
// A block it cannot take in whole is left, with the reason, and the manifest
// gets none of it. dryRun reports without saving.
func (r Routes) KeepEdits(emitUseKeychain, dryRun bool) (AdoptResult, error) {
	e := r.e
	var res AdoptResult
	m, err := e.load()
	if err != nil {
		return res, err
	}
	svc := configsvc.New(e.p.SSHDir, m, emitUseKeychain).WithState(e.p.StateDir())
	blocks, edits, err := svc.EditedBlocks()
	if err != nil {
		return res, err
	}
	for _, ed := range edits {
		if ed.Alias == "" {
			res.Left = append(res.Left, ed.String()+" - a comment under no host is not kept; put it on a host with --comment or --section")
		}
	}
	for _, eb := range blocks {
		b := eb.Block
		leave := func(why string) { res.Left = append(res.Left, fmt.Sprintf("%s: %s: %s", eb.File, b.Alias(), why)) }
		if b.Git {
			leave("the git ssh mode block is rendered from defaults.git; change that instead")
			continue
		}
		profile, err := profileFor(m, eb.File, b)
		if err != nil {
			leave(err.Error())
			continue
		}
		a, err := adoptBlock(m, profile, b)
		if err != nil {
			leave(err.Error())
			continue
		}
		a.File = eb.File
		res.Adopted = append(res.Adopted, a)
	}
	if len(res.Adopted) == 0 || dryRun {
		return res, nil
	}
	if err := e.save(m); err != nil {
		return res, err
	}
	names := make([]string, 0, len(res.Adopted))
	for _, a := range res.Adopted {
		names = append(names, a.Profile+"/"+a.Alias)
	}
	log.Audit(e.p.AuditLog(), "config.adopt_edits", log.Field{Key: "hosts", Value: strings.Join(names, ",")})
	return res, nil
}

// adoptBlock applies one block to m in place; m is unchanged on error.
func adoptBlock(m *manifest.Manifest, profile string, b renderer.Block) (Adopted, error) {
	a := Adopted{Profile: profile, Alias: b.Alias()}
	p := m.Profiles[profile]
	orig := p.Hosts
	hosts := append([]manifest.Host(nil), p.Hosts...)
	commit := func() error {
		p.Hosts = hosts
		m.SetProfile(profile, p)
		if err := m.Validate(); err != nil {
			p.Hosts = orig
			m.SetProfile(profile, p)
			return err
		}
		return nil
	}
	idx, variant, found := findAlias(m, profile, b.Alias())
	if !found {
		if vi, name, ok := asVariant(m, profile, b); ok {
			idx, variant, found = vi, name, true
			a.Action = "added variant " + name + " of " + hosts[vi].Alias
		}
	}
	switch {
	case found && variant != "":
		base := hosts[idx]
		kn, err := m.ResolvedKeyName(profile, base)
		if err != nil {
			return a, err
		}
		h, err := hostFromBlock(m, profile, b)
		if err != nil {
			return a, err
		}
		if file, _ := b.Get("IdentityFile"); strings.TrimSuffix(file, ".pub") != strings.TrimSuffix(m.IdentityFile(profile, kn), ".pub") {
			return a, fmt.Errorf("a variant uses its host's key, and this block names another (%s)", file)
		}
		if h.User != base.User {
			return a, fmt.Errorf("a variant uses its host's user %s, and this block has %s", base.User, h.User)
		}
		v := variantFrom(variant, base, h)
		base.Variants = append([]manifest.Variant(nil), base.Variants...)
		replaced := false
		for i := range base.Variants {
			if base.Variants[i].Name == variant {
				base.Variants[i], replaced = v, true
			}
		}
		if !replaced {
			base.Variants = append(base.Variants, v)
		} else if a.Action == "" {
			a.Action = "updated variant " + variant + " of " + base.Alias
		}
		hosts[idx] = base
	case found:
		cur := hosts[idx]
		h, err := hostFromBlock(m, profile, b)
		if err != nil {
			return a, err
		}
		key, err := keyFor(m, profile, cur.Alias, b)
		if err != nil {
			return a, err
		}
		if b.Alias() != cur.Alias {
			return a, fmt.Errorf("it is %s's extra name; edit the block named %s instead", cur.Alias, cur.Alias)
		}
		cur.Hostname, cur.User, cur.Port = h.Hostname, h.User, h.Port
		cur.Aliases, cur.Proxy, cur.RawOptions = h.Aliases, h.Proxy, h.RawOptions
		cur.Section, cur.Comment = h.Section, h.Comment
		if key != nil {
			cur.KeyName = key
		}
		hosts[idx] = cur
		a.Action = "updated host"
	default:
		h, err := hostFromBlock(m, profile, b)
		if err != nil {
			return a, err
		}
		if h.KeyName, err = keyFor(m, profile, h.Alias, b); err != nil {
			return a, err
		}
		hosts = append(hosts, h)
		a.Action = "added host"
	}
	if err := commit(); err != nil {
		return a, err
	}
	return a, nil
}
