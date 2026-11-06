// Package naming holds the rules for how servers and their keys are named:
// an alias template such as {code}-{env}-{role}-{nn}, from which an alias is
// checked, parsed into its parts or generated with the next free number, and a
// key-name template such as {profile}_{alias}-{algo}. It is pure: no file, no
// manifest, just templates and strings.
package naming

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The defaults: the convention authama documents, and the key name every
// release before templates derived.
const (
	DefaultAlias   = "{code}-{env}-{role}-{nn}"
	DefaultKeyName = "{profile}_{alias}-{algo}"
)

// Enforcement levels for an alias that does not follow the rules when a host
// is added. Hosts already in the manifest are only ever reported.
const (
	EnforceOff   = "off"
	EnforceWarn  = "warn"
	EnforceError = "error"
)

// aliasTokens are what an alias template may name; partValue is what each
// matches in an alias.
var aliasTokens = map[string]string{
	"code": `[a-z0-9]+`,
	"env":  `[a-z0-9]+`,
	"role": `[a-z0-9]+`,
	"nn":   `[0-9]{2,}`,
}

// keyTokens are what a key-name template may name.
var keyTokens = map[string]bool{"profile": true, "alias": true, "algo": true, "code": true, "env": true, "role": true, "nn": true}

var tokenRe = regexp.MustCompile(`\{([a-z]+)\}`)

// Rules are one manifest's naming rules, with the defaults filled in.
type Rules struct {
	Alias   string
	KeyName string
	Envs    []string
	Roles   []string
	Enforce string
}

// WithDefaults fills what r leaves empty.
func (r Rules) WithDefaults() Rules {
	if r.Alias == "" {
		r.Alias = DefaultAlias
	}
	if r.KeyName == "" {
		r.KeyName = DefaultKeyName
	}
	if r.Enforce == "" {
		r.Enforce = EnforceOff
	}
	return r
}

// segments splits a template into literal text and token names, in order.
func segments(tpl string) (lits []string, toks []string) {
	last := 0
	for _, m := range tokenRe.FindAllStringSubmatchIndex(tpl, -1) {
		lits = append(lits, tpl[last:m[0]])
		toks = append(toks, tpl[m[2]:m[3]])
		last = m[1]
	}
	lits = append(lits, tpl[last:])
	return lits, toks
}

var aliasLiteral = regexp.MustCompile(`^[a-z0-9.-]*$`)

// CheckAlias reports whether tpl can be an alias template: only known tokens,
// each at most once, literals that keep the alias a valid host name, no two
// tokens touching (they could not be told apart), and {nn}, so a second
// server of the same kind can be named.
func CheckAlias(tpl string) error {
	lits, toks := segments(tpl)
	if len(toks) == 0 {
		return fmt.Errorf("naming.alias %q names no part; use {code}, {env}, {role} and {nn}", tpl)
	}
	seen := map[string]bool{}
	for i, t := range toks {
		if _, ok := aliasTokens[t]; !ok {
			return fmt.Errorf("naming.alias %q: {%s} is not a part; use {code}, {env}, {role} or {nn}", tpl, t)
		}
		if seen[t] {
			return fmt.Errorf("naming.alias %q names {%s} twice", tpl, t)
		}
		seen[t] = true
		if i > 0 && lits[i] == "" {
			return fmt.Errorf("naming.alias %q: {%s} and {%s} touch; put a separator between them", tpl, toks[i-1], t)
		}
	}
	if !seen["nn"] {
		return fmt.Errorf("naming.alias %q has no {nn}: a second server of the same kind could not be named", tpl)
	}
	for _, l := range lits {
		if !aliasLiteral.MatchString(l) {
			return fmt.Errorf("naming.alias %q: %q is not allowed in a host name; use lowercase letters, digits, hyphens and dots", tpl, l)
		}
	}
	return nil
}

// CheckKeyName reports whether tpl can be a key-name template. A key name is
// read back in two places, so the template keeps their shape: it starts with
// {profile}_ (the profile is everything before the first underscore) and ends
// with -{algo} (the key type ssh-keygen is given).
func CheckKeyName(tpl string) error {
	if !strings.HasPrefix(tpl, "{profile}_") || !strings.HasSuffix(tpl, "-{algo}") {
		return fmt.Errorf("naming.key_name %q must start with {profile}_ and end with -{algo}, as in %s", tpl, DefaultKeyName)
	}
	_, toks := segments(tpl)
	for _, t := range toks {
		if !keyTokens[t] {
			return fmt.Errorf("naming.key_name %q: {%s} is not a part; use {profile}, {alias}, {code}, {env}, {role}, {nn} or {algo}", tpl, t)
		}
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(tpl, "{profile}_"), "-{algo}")
	if len(tokenRe.FindAllString(mid, -1)) == 0 {
		return fmt.Errorf("naming.key_name %q names nothing between the profile and the algorithm, so every host would share one key name", tpl)
	}
	if strings.Contains(tokenRe.ReplaceAllString(mid, ""), "/") {
		return fmt.Errorf("naming.key_name %q: a key name cannot hold /", tpl)
	}
	return nil
}

// Check reports whether r's templates, levels and lists are usable.
func (r Rules) Check() error {
	r = r.WithDefaults()
	if err := CheckAlias(r.Alias); err != nil {
		return err
	}
	if err := CheckKeyName(r.KeyName); err != nil {
		return err
	}
	switch r.Enforce {
	case EnforceOff, EnforceWarn, EnforceError:
	default:
		return fmt.Errorf("naming.enforce %q: use off, warn or error", r.Enforce)
	}
	for _, list := range [][]string{r.Envs, r.Roles} {
		for _, v := range list {
			if !regexp.MustCompile(`^` + aliasTokens["env"] + `$`).MatchString(v) {
				return fmt.Errorf("naming: %q is not a usable env or role; use lowercase letters and digits", v)
			}
		}
	}
	return nil
}

// pattern is tpl as an anchored regular expression with a named group per part.
func pattern(tpl string) *regexp.Regexp {
	lits, toks := segments(tpl)
	var b strings.Builder
	b.WriteString("^")
	for i, l := range lits {
		b.WriteString(regexp.QuoteMeta(l))
		if i < len(toks) {
			fmt.Fprintf(&b, "(?P<%s>%s)", toks[i], aliasTokens[toks[i]])
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// Parse reads alias's parts under r's alias template, or reports false when
// the alias does not follow it.
func (r Rules) Parse(alias string) (map[string]string, bool) {
	r = r.WithDefaults()
	if CheckAlias(r.Alias) != nil {
		return nil, false
	}
	re := pattern(r.Alias)
	m := re.FindStringSubmatch(alias)
	if m == nil {
		return nil, false
	}
	parts := map[string]string{}
	for i, n := range re.SubexpNames() {
		if n != "" {
			parts[n] = m[i]
		}
	}
	return parts, true
}

// Violation says why alias does not follow r for a profile whose code is
// code, or "" when it does.
func (r Rules) Violation(alias, code string) string {
	r = r.WithDefaults()
	parts, ok := r.Parse(alias)
	if !ok {
		return fmt.Sprintf("%s does not follow %s", alias, r.Alias)
	}
	if code != "" && parts["code"] != "" && parts["code"] != code {
		return fmt.Sprintf("%s starts with %s, but its profile's code is %s", alias, parts["code"], code)
	}
	if len(r.Envs) > 0 && parts["env"] != "" && !contains(r.Envs, parts["env"]) {
		return fmt.Sprintf("%s: env %s is not one of %s", alias, parts["env"], strings.Join(r.Envs, ", "))
	}
	if len(r.Roles) > 0 && parts["role"] != "" && !contains(r.Roles, parts["role"]) {
		return fmt.Sprintf("%s: role %s is not one of %s", alias, parts["role"], strings.Join(r.Roles, ", "))
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Generate makes the next alias for parts (code, env, role) under r: the
// lowest {nn}, from 00, that no name in taken already has with the same
// other parts.
func (r Rules) Generate(parts map[string]string, taken []string) (string, error) {
	r = r.WithDefaults()
	if err := CheckAlias(r.Alias); err != nil {
		return "", err
	}
	_, toks := segments(r.Alias)
	for _, t := range toks {
		if t == "nn" {
			continue
		}
		v := parts[t]
		if v == "" {
			return "", fmt.Errorf("naming the server needs its %s (%s)", t, r.Alias)
		}
		if !regexp.MustCompile(`^` + aliasTokens[t] + `$`).MatchString(v) {
			return "", fmt.Errorf("%s %q: use lowercase letters and digits", t, v)
		}
	}
	if len(r.Envs) > 0 && parts["env"] != "" && !contains(r.Envs, parts["env"]) {
		return "", fmt.Errorf("env %s is not one of %s", parts["env"], strings.Join(r.Envs, ", "))
	}
	if len(r.Roles) > 0 && parts["role"] != "" && !contains(r.Roles, parts["role"]) {
		return "", fmt.Errorf("role %s is not one of %s", parts["role"], strings.Join(r.Roles, ", "))
	}
	used := map[int]bool{}
	for _, a := range taken {
		p, ok := r.Parse(a)
		if !ok {
			continue
		}
		same := true
		for _, t := range toks {
			if t != "nn" && p[t] != parts[t] {
				same = false
			}
		}
		if n, err := strconv.Atoi(p["nn"]); same && err == nil {
			used[n] = true
		}
	}
	n := 0
	for used[n] {
		n++
	}
	fill := map[string]string{"nn": fmt.Sprintf("%02d", n)}
	for _, t := range toks {
		if t != "nn" {
			fill[t] = parts[t]
		}
	}
	return render(r.Alias, fill), nil
}

func render(tpl string, vals map[string]string) string {
	return tokenRe.ReplaceAllStringFunc(tpl, func(m string) string { return vals[m[1:len(m)-1]] })
}

// ErrNoParts is a key-name template naming alias parts for an alias that does
// not follow the alias template; the caller falls back to the default name.
var ErrNoParts = errors.New("the alias does not follow the alias template, so its parts are unknown")

// RenderKeyName renders r's key-name template for a host. profile, alias and code
// arrive normalised by the caller; the parts come from parsing alias. A
// template that names a part the alias does not have is ErrNoParts.
func (r Rules) RenderKeyName(profile, alias, code, algo string) (string, error) {
	r = r.WithDefaults()
	vals := map[string]string{"profile": profile, "alias": alias, "algo": algo, "code": code}
	_, toks := segments(r.KeyName)
	needParts := false
	for _, t := range toks {
		if t == "env" || t == "role" || t == "nn" || (t == "code" && code == "") {
			needParts = true
		}
	}
	if needParts {
		parts, ok := r.Parse(alias)
		if !ok {
			return "", ErrNoParts
		}
		for k, v := range parts {
			if k != "code" || vals["code"] == "" {
				vals[k] = v
			}
		}
	}
	return render(r.KeyName, vals), nil
}

// Tokens lists the parts tpl names, sorted.
func Tokens(tpl string) []string {
	_, toks := segments(tpl)
	sort.Strings(toks)
	return toks
}
