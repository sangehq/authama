package tokens

import (
	"embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// A token config maps each host to how its credential is built from the
// environment. It holds env var NAMES, never secrets, so it is safe to commit.
// Three shapes are accepted: v2 grouped by target ({"npm": {...}}), the legacy
// {"credentials": {host: spec}} and a bare {host: spec}; the last two are
// Composer-only.

// Config is a parsed token config, in file order.
type Config struct{ root *object }

// ParseConfig reads a token config; it must be a JSON object.
func ParseConfig(text string) (*Config, error) {
	v, err := decodeOrdered(text)
	if err != nil {
		return nil, err
	}
	o, ok := v.(*object)
	if !ok {
		return nil, errors.New("must be a JSON object")
	}
	return &Config{root: o}, nil
}

// NewConfig is an empty config.
func NewConfig() *Config { return &Config{root: newObject()} }

// JSON renders the config as json.dumps(indent=2) did.
func (c *Config) JSON() string { return encodeIndented(c.root) }

// Setting reads a string from the optional "_settings" object.
func (c *Config) Setting(name string) string {
	s, ok := c.root.vals["_settings"].(*object)
	if !ok {
		return ""
	}
	v, _ := s.vals[name].(string)
	return v
}

// Spec is one host's entry, with the fields a config may set.
type Spec struct {
	Target     string
	Host       string
	Type       string
	Service    string
	Env        string
	UserEnv    string
	PassEnv    string
	KeyEnv     string
	SecretEnv  string
	Headers    []string
	Scope      string
	AlwaysAuth bool
	present    map[string]bool
	malformed  bool
}

// Has reports whether the entry set a field at all.
func (s Spec) Has(field string) bool { return s.present[field] }

// Specs lists every entry, normalised to v2, in target then file order.
func (c *Config) Specs() []Spec {
	var out []Spec
	isV2 := false
	for _, t := range Targets() {
		if _, ok := c.root.get(t); ok {
			isV2 = true
		}
	}
	if isV2 {
		for _, t := range Targets() {
			hosts, ok := c.root.vals[t].(*object)
			if !ok {
				continue
			}
			for _, h := range hosts.keys {
				out = append(out, specOf(t, h, hosts.vals[h]))
			}
		}
		return out
	}
	creds := c.root
	if v, ok := c.root.get("credentials"); ok {
		o, isObj := v.(*object)
		if !isObj {
			return nil
		}
		creds = o
	}
	for _, h := range creds.keys {
		if !strings.HasPrefix(h, "_") {
			out = append(out, specOf(Composer, h, creds.vals[h]))
		}
	}
	return out
}

func specOf(target, host string, v any) Spec {
	s := Spec{Target: target, Host: host, present: map[string]bool{}}
	o, ok := v.(*object)
	if !ok {
		s.malformed = true
		return s
	}
	str := func(k string) string {
		x, _ := o.vals[k].(string)
		return x
	}
	for _, k := range o.keys {
		s.present[k] = true
	}
	s.Type, s.Service, s.Env = str("type"), str("service"), str("env")
	s.UserEnv, s.PassEnv = str("user_env"), str("pass_env")
	s.KeyEnv, s.SecretEnv = str("key_env"), str("secret_env")
	s.Scope = str("scope")
	s.AlwaysAuth = truthy(o.vals["always_auth"])
	if hs, ok := o.vals["headers"].([]any); ok {
		for _, h := range hs {
			s.Headers = append(s.Headers, pyStr(h))
		}
	}
	return s
}

// Mechanism is the Composer mechanism for a spec: an explicit type wins,
// else the service alias maps to one.
func Mechanism(s Spec) string {
	if s.Type != "" {
		return s.Type
	}
	return cat().Services[s.Service]
}

// Lookup reads an environment variable, reporting whether it is set.
type Lookup func(string) (string, bool)

func envValue(get Lookup, name string) string {
	if name == "" {
		return ""
	}
	v, _ := get(name)
	return v
}

var envRef = regexp.MustCompile(`\$\{(\w+)\}`)

// ExpandEnv substitutes ${VAR}; ok is false if a referenced var is absent
// (a var that is set but empty substitutes as empty).
func ExpandEnv(text string, get Lookup) (string, bool) {
	ok := true
	out := envRef.ReplaceAllStringFunc(text, func(m string) string {
		v, set := get(m[2 : len(m)-1])
		if !set {
			ok = false
		}
		return v
	})
	return out, ok
}

// Resolve builds a credential set from the config's references, reading the
// secrets through get. Entries that cannot be built are skipped, and why is
// recorded in the set's Warnings.
func (c *Config) Resolve(get Lookup) *Set {
	s := NewSet()
	for _, spec := range c.Specs() {
		s.resolveOne(spec, get)
	}
	return s
}

func (s *Set) resolveOne(sp Spec, get Lookup) {
	skip := func(why string) {
		s.Warnings = append(s.Warnings, fmt.Sprintf("skip %s:%s: %s", sp.Target, sp.Host, why))
	}
	if sp.malformed {
		skip("entry is not a JSON object")
		return
	}
	c := Credential{Target: sp.Target, Host: sp.Host}
	switch sp.Target {
	case Composer:
		mech := Mechanism(sp)
		if mech == "" {
			if sp.Service != "" {
				skip(fmt.Sprintf("unknown service %s", pyRepr(sp.Service)))
			} else {
				skip("missing 'type'/'service'")
			}
			return
		}
		c.Mechanism = mech
		switch {
		case mech == "http-basic":
			c.Kind, c.Username, c.Password = KindBasic, envValue(get, sp.UserEnv), envValue(get, sp.PassEnv)
		case mech == "bitbucket-oauth":
			c.Kind, c.Key, c.Secret = KindPair, envValue(get, sp.KeyEnv), envValue(get, sp.SecretEnv)
		case mech == "custom-headers":
			c.Kind = KindHeaders
			all := true
			var hs []string
			for _, h := range sp.Headers {
				v, ok := ExpandEnv(h, get)
				if !ok {
					all = false
				}
				hs = append(hs, v)
			}
			if all {
				c.Headers = hs
			}
		case cat().TokenMechanisms[mech]:
			c.Kind, c.Token = KindToken, envValue(get, sp.Env)
		default:
			// The Python tool reported both lines here; kept for parity.
			skip(fmt.Sprintf("unsupported type %s", pyRepr(mech)))
			skip("required env var(s) not set")
			return
		}
	case NPM:
		c.Kind, c.Token, c.Scope, c.AlwaysAuth = KindToken, envValue(get, sp.Env), sp.Scope, sp.AlwaysAuth
	case Docker, Git, Netrc:
		c.Kind, c.Username, c.Password = KindBasic, envValue(get, sp.UserEnv), envValue(get, sp.PassEnv)
	default:
		skip(fmt.Sprintf("unknown target %s", pyRepr(sp.Target)))
		return
	}
	trimmed := c
	trimmed.Token, trimmed.Username, trimmed.Password = strings.TrimSpace(c.Token), strings.TrimSpace(c.Username), strings.TrimSpace(c.Password)
	trimmed.Key, trimmed.Secret = strings.TrimSpace(c.Key), strings.TrimSpace(c.Secret)
	if trimmed.complete() {
		if err := Check(trimmed); err != nil {
			skip(err.Error())
			return
		}
	}
	if !s.Add(c) {
		skip("required env var(s) not set")
	}
}

// pyRepr quotes a string the way Python's repr does for plain text.
func pyRepr(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
}

// RelevantWarnings filters resolution warnings to the selected targets (all
// when none are selected) and, past six "env var not set" skips, folds those
// into one summary line at the end, as the Python tool did: a starter config
// lists dozens of example hosts and a wall of identical lines hides the
// mistakes that matter.
func RelevantWarnings(warnings []string, selected map[string]bool) []string {
	const suffix = "required env var(s) not set"
	var relevant, unset []string
	for _, w := range warnings {
		keep := len(selected) == 0
		for t := range selected {
			if strings.HasPrefix(w, "skip "+t+":") {
				keep = true
			}
		}
		if keep {
			relevant = append(relevant, w)
			if strings.HasSuffix(w, suffix) {
				unset = append(unset, w)
			}
		}
	}
	if len(unset) <= 6 {
		return relevant
	}
	var out []string
	for _, w := range relevant {
		if !strings.HasSuffix(w, suffix) {
			out = append(out, w)
		}
	}
	counts := map[string]int{}
	var order []string
	for _, w := range unset {
		f := pyFields(w)
		t, _, _ := strings.Cut(f[1], ":")
		if counts[t] == 0 {
			order = append(order, t)
		}
		counts[t]++
	}
	parts := make([]string, len(order))
	for i, t := range order {
		parts[i] = fmt.Sprintf("%s %d", t, counts[t])
	}
	return append(out, fmt.Sprintf("skipped %d config entries whose env vars are not set (%s) — "+
		"normal if you only use some hosts; trim the config or set the vars.", len(unset), strings.Join(parts, ", ")))
}

// ValidateEntry reports the fields a spec is missing for its target, or "".
func ValidateEntry(sp Spec) string {
	fields := cat().TargetFields
	var required []string
	if sp.Target == Composer {
		mech := Mechanism(sp)
		if mech == "" {
			return "needs a 'type' or known 'service'"
		}
		var ok bool
		if required, ok = fields[Composer][mech]; !ok {
			required = fields[Composer]["token"]
		}
	} else {
		f, ok := fields[sp.Target]
		if !ok || len(f) == 0 {
			return fmt.Sprintf("unknown target %s", pyRepr(sp.Target))
		}
		for _, v := range f {
			required = v
		}
	}
	var missing []string
	for _, f := range required {
		if !sp.Has(f) {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return "missing field(s): " + strings.Join(missing, ", ")
	}
	return ""
}

//go:embed templates/*.json
var templateFS embed.FS

// AllTargetsTemplate is the shipped all-targets example, byte for byte.
func AllTargetsTemplate() string {
	b, _ := templateFS.ReadFile("templates/all-targets.example.json")
	return string(b)
}

// TargetTemplate is the starter template for one target.
func TargetTemplate(target string) (*Config, error) {
	if !IsTarget(target) {
		return nil, fmt.Errorf("no template for unknown target %s", pyRepr(target))
	}
	b, err := templateFS.ReadFile("templates/" + target + ".example.json")
	if err != nil {
		return nil, fmt.Errorf("template for %s not found", pyRepr(target))
	}
	return ParseConfig(string(b))
}

// StarterHelp heads a config scaffolded for chosen targets.
const StarterHelp = "Token config — maps each host to how its credential is built from the " +
	"environment, grouped by TARGET tool. Stores ENV VAR NAMES, not secrets. " +
	"See `authama token skeleton` for a reference and `authama token services` for " +
	"Composer services. Delete the hosts you don't use."

// StarterConfig assembles a config from the per-target templates.
func StarterConfig(targets []string) (*Config, error) {
	c := NewConfig()
	c.root.set("_help", StarterHelp)
	for _, t := range targets {
		tpl, err := TargetTemplate(t)
		if err != nil {
			return nil, err
		}
		v, ok := tpl.root.get(t)
		if !ok {
			v = newObject()
		}
		c.root.set(t, v)
	}
	return c, nil
}

// SetEntry adds or replaces one host's reference entry in the v2 shape,
// keeping the rest of the file in order. fields is ordered key/value pairs.
func (c *Config) SetEntry(target, host string, fields [][2]string) {
	sec, ok := c.root.vals[target].(*object)
	if !ok {
		sec = newObject()
		c.root.set(target, sec)
	}
	e := newObject()
	for _, kv := range fields {
		e.set(kv[0], kv[1])
	}
	sec.set(host, e)
}

// ParseEnvFile reads VAR=value lines, as --source did: blank lines and
// comments are skipped and one layer of quotes is stripped.
func ParseEnvFile(text string) [][2]string {
	var out [][2]string
	for _, line := range pySplitLines(text) {
		line = pyStrip(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		v = strings.Trim(strings.Trim(pyStrip(v), `"`), "'")
		out = append(out, [2]string{pyStrip(k), v})
	}
	return out
}

// Template is the complete reference skeleton: every target and mechanism,
// with placeholders that are never valid credentials.
func Template() *Set {
	s := NewSet()
	tok := func(host, mech, v string) {
		s.Add(Credential{Target: Composer, Host: host, Kind: KindToken, Mechanism: mech, Token: v})
	}
	basic := func(target, host, mech, u, p string) {
		s.Add(Credential{Target: target, Host: host, Kind: KindBasic, Mechanism: mech, Username: u, Password: p})
	}
	tok("github.com", "github-oauth", "<github-personal-access-token>")
	tok("gitlab.example.org", "gitlab-oauth", "<gitlab-oauth-token>")
	tok("gitlab.com", "gitlab-token", "<gitlab-personal-or-deploy-token>")
	s.Add(Credential{Target: Composer, Host: "bitbucket.org", Kind: KindPair, Mechanism: "bitbucket-oauth", Key: "<consumer-key>", Secret: "<consumer-secret>"})
	basic(Composer, "repo.packagist.com", "http-basic", "<username>", "<password>")
	basic(Composer, "repo.magento.com", "http-basic", "<magento-public-key>", "<magento-private-key>")
	tok("private.example.org", "bearer", "<bearer-token>")
	s.Add(Credential{Target: Composer, Host: "repo.example.org", Kind: KindHeaders, Mechanism: "custom-headers", Headers: []string{"API-TOKEN: <token>", "X-Org: acme"}})
	tok("code.example.org", "forgejo-token", "<forgejo-token>")
	s.Add(Credential{Target: NPM, Host: "registry.npmjs.org", Kind: KindToken, Token: "<npm-token>"})
	s.Add(Credential{Target: NPM, Host: "npm.pkg.github.com", Kind: KindToken, Token: "<github-token>", Scope: "@your-org"})
	basic(Docker, "ghcr.io", "", "<docker-username>", "<docker-token>")
	basic(Git, "github.com", "", "<git-username>", "<git-token>")
	basic(Netrc, "api.heroku.com", "", "<login>", "<api-key>")
	return s
}

// Apply adds one interactively entered credential: token mechanisms use
// token (+ scope for npm); basic ones username + password; bitbucket key +
// secret; custom-headers headers. It reports whether it was kept.
func (s *Set) Apply(target, mechanism, host string, c Credential) bool {
	c.Target, c.Host = target, host
	switch target {
	case Composer:
		c.Mechanism = mechanism
		switch {
		case cat().TokenMechanisms[mechanism]:
			c.Kind = KindToken
		case mechanism == "http-basic":
			c.Kind = KindBasic
		case mechanism == "bitbucket-oauth":
			c.Kind = KindPair
		case mechanism == "custom-headers":
			c.Kind = KindHeaders
		default:
			return false
		}
	case NPM:
		c.Kind = KindToken
		c.AlwaysAuth = false
	case Docker, Git, Netrc:
		c.Kind = KindBasic
	default:
		return false
	}
	return s.Add(c)
}
