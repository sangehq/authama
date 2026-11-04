// Package tokens renders, reads back and removes the credentials that package
// managers and VCS clients read from their own files: Composer's auth.json,
// .npmrc, Docker's config.json, git's credential store and .netrc.
//
// It is the Go port of simtabi/auth-token-manager (see CHANGELOG for the exact
// revision). The package is pure: it turns config and file text into file
// text, and never touches the filesystem or the environment itself; the
// services layer (internal/services/tokensvc) does the I/O, sandboxing and
// permission hardening. The output of every renderer, merge, scanner and purge
// is byte-compatible with the Python tool, which golden tests pin.
package tokens

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
)

//go:embed catalog.json
var catalogJSON string

// The five targets, in their documented order.
const (
	Composer = "composer"
	NPM      = "npm"
	Docker   = "docker"
	Git      = "git"
	Netrc    = "netrc"
)

// Provider is one row of the interactive provider picker for a target.
type Provider struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Cloud    string `json:"cloud"`
	Hostable bool   `json:"hostable"`
}

// EnterpriseHost says whether a Composer service has a public cloud host and
// which composer.json config key lists its self-hosted domains.
type EnterpriseHost struct {
	Cloud   string // "" when self-hosted by nature
	Domains string // "" when Composer needs no *-domains entry
}

// MenuItem is one entry of the "which tool?" menu.
type MenuItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Catalog is the data the Python tool kept in config/catalog.json: targets,
// Composer services, enterprise hosts, providers and required config fields.
// Adding a service or provider is an edit to catalog.json, not to code.
type Catalog struct {
	Targets         []string
	Menu            []MenuItem
	TargetFields    map[string]map[string][]string
	TokenMechanisms map[string]bool // Composer mechanisms that store one token
	Services        map[string]string
	serviceOrder    []string
	Enterprise      map[string]EnterpriseHost
	enterpriseOrder []string
	Providers       map[string][]Provider
	providerOrder   []string
	DefaultConfig   string // the built-in minimal config, as JSON text
}

var embeddedCatalog, catalogErr = parseCatalog(catalogJSON)

// override is a catalog the user supplied in place of the embedded one (see
// UseCatalog); nil means the embedded catalog is in use.
var override atomic.Pointer[Catalog]

func current() *Catalog {
	if c := override.Load(); c != nil {
		return c
	}
	return embeddedCatalog
}

// LoadCatalog returns the catalog in use: one installed by UseCatalog, or the
// embedded one (a test pins that the embedded one always reads).
func LoadCatalog() (*Catalog, error) {
	if c := current(); c != nil {
		return c, nil
	}
	return nil, catalogErr
}

// cat is the catalog for package-internal use; a broken embed is caught by
// TestTheCatalogLoads, and every caller degrades to an empty catalog.
func cat() *Catalog {
	if c := current(); c != nil {
		return c
	}
	return &Catalog{}
}

// UseCatalog replaces the embedded catalog with text, a catalog.json the user
// supplied (auth-token-manager read one from $AUTH_TOKEN_MANAGER_CATALOG), so
// a new Composer service or provider can be added without a new build. Empty
// text restores the embedded catalog. A catalog that does not parse, or names
// a target this package cannot write, is refused and the current one is kept.
func UseCatalog(text string) error {
	if text == "" {
		override.Store(nil)
		return nil
	}
	c, err := parseCatalog(text)
	if err != nil {
		return err
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("token catalog: no targets")
	}
	known := map[string]bool{Composer: true, NPM: true, Docker: true, Git: true, Netrc: true}
	names := []string{Composer, NPM, Docker, Git, Netrc}
	for _, t := range []string{PyPI, Cargo, Gem, Gradle} {
		known[t], names = true, append(names, t)
	}
	for _, t := range c.Targets {
		if !known[t] {
			return fmt.Errorf("token catalog: unknown target %q (choose from %s)", t, strings.Join(names, ", "))
		}
	}
	override.Store(c)
	return nil
}

func parseCatalog(text string) (*Catalog, error) {
	var raw struct {
		Targets      []string                       `json:"targets"`
		Menu         []MenuItem                     `json:"menu"`
		TargetFields map[string]map[string][]string `json:"target_fields"`
		Default      json.RawMessage                `json:"default_config"`
		Composer     struct {
			TokenMethods map[string]string `json:"token_methods"`
			Services     map[string]string `json:"services"`
			Enterprise   map[string]struct {
				Cloud   *string `json:"cloud"`
				Domains *string `json:"domains"`
			} `json:"enterprise_hosts"`
		} `json:"composer"`
		Providers map[string][]Provider `json:"providers"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("token catalog: %w", err)
	}
	c := &Catalog{
		Targets:         raw.Targets,
		Menu:            raw.Menu,
		TargetFields:    raw.TargetFields,
		TokenMechanisms: map[string]bool{},
		Services:        raw.Composer.Services,
		Enterprise:      map[string]EnterpriseHost{},
		Providers:       raw.Providers,
		DefaultConfig:   string(raw.Default),
	}
	for m := range raw.Composer.TokenMethods {
		c.TokenMechanisms[m] = true
	}
	for k, v := range raw.Composer.Enterprise {
		var e EnterpriseHost
		if v.Cloud != nil {
			e.Cloud = *v.Cloud
		}
		if v.Domains != nil {
			e.Domains = *v.Domains
		}
		c.Enterprise[k] = e
	}
	// Key order matters (the first provider to name a host labels it), and
	// encoding/json forgets it, so read the orders separately.
	doc, err := decodeOrdered(text)
	if err != nil {
		return nil, fmt.Errorf("token catalog: %w", err)
	}
	root, _ := doc.(*object)
	if root == nil {
		return nil, fmt.Errorf("token catalog: not an object")
	}
	c.providerOrder = keysOf(root, "providers")
	if comp, ok := root.get("composer"); ok {
		if co, ok := comp.(*object); ok {
			c.serviceOrder = keysOf(co, "services")
			c.enterpriseOrder = keysOf(co, "enterprise_hosts")
		}
	}
	return c, nil
}

func keysOf(o *object, key string) []string {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	if sub, ok := v.(*object); ok {
		return append([]string(nil), sub.keys...)
	}
	return nil
}

// ServiceNames lists the Composer service aliases in catalog order.
func (c *Catalog) ServiceNames() []string { return append([]string(nil), c.serviceOrder...) }

// EnterpriseNames lists the services that can be cloud or self-hosted.
func (c *Catalog) EnterpriseNames() []string { return append([]string(nil), c.enterpriseOrder...) }

// ProviderTargets lists the targets with an interactive provider list.
func (c *Catalog) ProviderTargets() []string { return append([]string(nil), c.providerOrder...) }

// IsTarget reports whether t is one of the five targets.
func IsTarget(t string) bool {
	for _, x := range cat().Targets {
		if x == t {
			return true
		}
	}
	return false
}

// Targets lists the five targets in order.
func Targets() []string { return append([]string(nil), cat().Targets...) }

// Mechanisms lists every Composer mechanism (auth.json section) the catalog
// knows, sorted.
func Mechanisms() []string {
	seen := map[string]bool{}
	for m := range cat().TokenMechanisms {
		seen[m] = true
	}
	for _, m := range cat().Services {
		seen[m] = true
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// hostLabels maps a host to a friendly provider label: providers' cloud hosts
// first, then enterprise cloud hosts titled from the service name.
func hostLabels() map[string]string {
	c := cat()
	labels := map[string]string{}
	for _, t := range c.providerOrder {
		for _, p := range c.Providers[t] {
			if p.Cloud != "" {
				if _, ok := labels[p.Cloud]; !ok {
					labels[p.Cloud] = p.Label
				}
			}
		}
	}
	for _, svc := range c.enterpriseOrder {
		cloud := c.Enterprise[svc].Cloud
		if cloud == "" {
			continue
		}
		if _, ok := labels[cloud]; !ok {
			labels[cloud] = pyTitle(strings.ReplaceAll(svc, "-", " "))
		}
	}
	return labels
}

// pyTitle is Python's str.title for ASCII: a letter after a non-letter is
// upper-cased, every other letter lower-cased.
func pyTitle(s string) string {
	var b strings.Builder
	prevLetter := false
	for _, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		switch {
		case isLetter && !prevLetter:
			b.WriteString(strings.ToUpper(string(r)))
		case isLetter:
			b.WriteString(strings.ToLower(string(r)))
		default:
			b.WriteRune(r)
		}
		prevLetter = isLetter
	}
	return b.String()
}

var targetRole = map[string]string{
	Composer: "Composer auth (PHP packages)",
	NPM:      "npm registry token",
	Docker:   "Docker registry login",
	Git:      "Git over HTTPS",
	Netrc:    "netrc machine login",
}

// Describe is a short human summary: the provider, if known, and what the
// credential is for, e.g. "GitHub · Git over HTTPS".
func Describe(target, host string) string {
	labels := hostLabels()
	provider := labels[host]
	if provider == "" {
		provider = labels[strings.SplitN(host, "/", 2)[0]]
	}
	if provider == "" {
		provider = host
	}
	role, ok := targetRole[target]
	if !ok {
		role = target
	}
	if provider != role {
		return provider + " · " + role
	}
	return role
}

// LoadCatalogServices is the Composer service -> mechanism map.
func LoadCatalogServices() map[string]string {
	out := map[string]string{}
	for k, v := range cat().Services {
		out[k] = v
	}
	return out
}

// IsTokenMechanism reports whether a Composer mechanism stores one token.
func IsTokenMechanism(m string) bool { return cat().TokenMechanisms[m] }
