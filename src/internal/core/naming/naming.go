// Package naming holds the rules for how servers and their keys are named:
// an alias template such as {code}-{env}-{role}-{nn}, from which an alias is
// checked, parsed into its parts or generated with the next free number, and a
// key-name template such as {profile}_{alias}-{algo}. It is pure: no file, no
// manifest, just templates and strings.
//
// A template is literal text and parts in braces:
//
//	{code} {env} {role}   lowercase letters and digits
//	{nn} {nnn} ...        the number, as many digits as n's (00, 000), from Start
//	{env?}                optional: left out, with the separator before it (or
//	                      after it, for the first part), when it has no value
//
// A key-name template may also name {profile}, {alias} and {algo}.
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

// What a profile with no code of its own uses for {code}.
const (
	// CodeNone: nothing; a template that needs {code} cannot name its
	// servers, and an optional {code?} is left out.
	CodeNone = "none"
	// CodeProfile: the profile's name, lowercased to letters and digits.
	CodeProfile = "profile"
)

const partValue = `[a-z0-9]+`

var tokenRe = regexp.MustCompile(`\{([a-z]+)(\?)?\}`)

var numberToken = regexp.MustCompile(`^n+$`)

// token is one part of a template.
type token struct {
	name     string // code, env, role, nn (the number, whatever its width), profile, alias, algo
	width    int    // digits, for the number
	optional bool
}

// Rules are one manifest's (or one profile's) naming rules, with the defaults
// filled in by WithDefaults.
type Rules struct {
	Alias        string
	KeyName      string
	Envs         []string
	Roles        []string
	Enforce      string
	Start        int    // the first number: 0 (00) or 1 (01)
	CodeFallback string // CodeNone or CodeProfile
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
	if r.CodeFallback == "" {
		r.CodeFallback = CodeNone
	}
	return r
}

// Over is r with every field over sets replacing r's: a profile's own rules
// over the manifest's.
func (r Rules) Over(over Rules) Rules {
	if over.Alias != "" {
		r.Alias = over.Alias
	}
	if over.KeyName != "" {
		r.KeyName = over.KeyName
	}
	if over.Envs != nil {
		r.Envs = over.Envs
	}
	if over.Roles != nil {
		r.Roles = over.Roles
	}
	if over.Enforce != "" {
		r.Enforce = over.Enforce
	}
	if over.Start != 0 {
		r.Start = over.Start
	}
	if over.CodeFallback != "" {
		r.CodeFallback = over.CodeFallback
	}
	return r
}

// parse splits a template into literal text and parts, in order.
func parse(tpl string) (lits []string, toks []token) {
	last := 0
	for _, m := range tokenRe.FindAllStringSubmatchIndex(tpl, -1) {
		lits = append(lits, tpl[last:m[0]])
		t := token{name: tpl[m[2]:m[3]], optional: m[4] >= 0}
		if numberToken.MatchString(t.name) {
			t.width, t.name = len(t.name), "nn"
		}
		toks = append(toks, t)
		last = m[1]
	}
	lits = append(lits, tpl[last:])
	return lits, toks
}

var aliasLiteral = regexp.MustCompile(`^[a-z0-9.-]*$`)

var aliasParts = map[string]bool{"code": true, "env": true, "role": true, "nn": true}

var keyParts = map[string]bool{"profile": true, "alias": true, "algo": true, "code": true, "env": true, "role": true, "nn": true}

// CheckAlias reports whether tpl can be an alias template: only {code},
// {env}, {role} and a number, each at most once; a separator between parts so
// they can be told apart; literals that keep the alias a valid host name; and
// a number that is not optional, so a second server of the same kind can be
// named.
func CheckAlias(tpl string) error {
	lits, toks := parse(tpl)
	if len(toks) == 0 {
		return fmt.Errorf("naming.alias %q names no part; use {code}, {env}, {role} and {nn}", tpl)
	}
	seen := map[string]bool{}
	for i, t := range toks {
		if !aliasParts[t.name] {
			return fmt.Errorf("naming.alias %q: a part is not {code}, {env}, {role} or a number such as {nn}", tpl)
		}
		if seen[t.name] {
			return fmt.Errorf("naming.alias %q names {%s} twice", tpl, t.name)
		}
		seen[t.name] = true
		if i > 0 && lits[i] == "" {
			return fmt.Errorf("naming.alias %q: two parts touch; put a separator between them", tpl)
		}
		if t.name == "nn" && t.optional {
			return fmt.Errorf("naming.alias %q: the number cannot be optional", tpl)
		}
		if t.optional && lits[i] == "" && lits[i+1] == "" {
			return fmt.Errorf("naming.alias %q: an optional part needs a separator beside it to leave out with it", tpl)
		}
		if t.optional && i > 0 && toks[i-1].optional {
			return fmt.Errorf("naming.alias %q: two optional parts in a row cannot be told apart when only one is there", tpl)
		}
	}
	if !seen["nn"] {
		return fmt.Errorf("naming.alias %q has no number such as {nn}: a second server of the same kind could not be named", tpl)
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
	_, toks := parse(tpl)
	for _, t := range toks {
		if !keyParts[t.name] {
			return fmt.Errorf("naming.key_name %q: a part is not {profile}, {alias}, {code}, {env}, {role}, a number or {algo}", tpl)
		}
	}
	mid := strings.TrimSuffix(strings.TrimPrefix(tpl, "{profile}_"), "-{algo}")
	_, midToks := parse(mid)
	required := 0
	for _, t := range midToks {
		if !t.optional {
			required++
		}
	}
	if required == 0 {
		return fmt.Errorf("naming.key_name %q names nothing certain between the profile and the algorithm, so hosts could share one key name", tpl)
	}
	if strings.Contains(tokenRe.ReplaceAllString(mid, ""), "/") {
		return fmt.Errorf("naming.key_name %q: a key name cannot hold /", tpl)
	}
	return nil
}

var valueRe = regexp.MustCompile(`^` + partValue + `$`)

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
	switch r.CodeFallback {
	case CodeNone, CodeProfile:
	default:
		return fmt.Errorf("naming.code_fallback %q: use none or profile", r.CodeFallback)
	}
	if r.Start < 0 || r.Start > 1 {
		return fmt.Errorf("naming.start %d: numbering starts at 0 or 1", r.Start)
	}
	for _, list := range [][]string{r.Envs, r.Roles} {
		for _, v := range list {
			if !valueRe.MatchString(v) {
				return fmt.Errorf("naming: %q is not a usable env or role; use lowercase letters and digits", v)
			}
		}
	}
	return nil
}

// Code is the {code} for a profile under r: its own code, else, with
// CodeFallback profile, its name lowercased to letters and digits.
func (r Rules) Code(profile, own string) string {
	if own != "" {
		return own
	}
	if r.WithDefaults().CodeFallback == CodeProfile {
		return strings.Map(func(c rune) rune {
			switch {
			case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
				return c
			case c >= 'A' && c <= 'Z':
				return c + 'a' - 'A'
			}
			return -1
		}, profile)
	}
	return ""
}

// pattern is an alias template as an anchored regular expression with a
// named group per part; an optional part's group takes its separator with it.
// values, when it lists a part's allowed values, narrows that part to them.
func pattern(tpl string, values map[string][]string) *regexp.Regexp {
	lits, toks := parse(tpl)
	var b strings.Builder
	b.WriteString("^")
	for i, t := range toks {
		val := partValue
		if t.name == "nn" {
			val = fmt.Sprintf(`[0-9]{%d,}`, t.width)
		} else if list := values[t.name]; len(list) > 0 {
			quoted := make([]string, len(list))
			for j, v := range list {
				quoted[j] = regexp.QuoteMeta(v)
			}
			val = "(?:" + strings.Join(quoted, "|") + ")"
		}
		group := fmt.Sprintf("(?P<%s>%s)", t.name, val)
		switch {
		case !t.optional:
			b.WriteString(regexp.QuoteMeta(lits[i]) + group)
		case lits[i] != "":
			b.WriteString("(?:" + regexp.QuoteMeta(lits[i]) + group + ")?")
		default: // first part: its separator comes after it
			b.WriteString("(?:" + group + regexp.QuoteMeta(lits[i+1]) + ")?")
			lits[i+1] = ""
		}
	}
	b.WriteString(regexp.QuoteMeta(lits[len(lits)-1]))
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// render fills tpl with vals; an optional part with no value is left out with
// its separator, as pattern reads it.
func render(tpl string, vals map[string]string) string {
	lits, toks := parse(tpl)
	var b strings.Builder
	for i, t := range toks {
		v := vals[t.name]
		switch {
		case v != "" || !t.optional:
			b.WriteString(lits[i] + v)
		case lits[i] == "": // a first optional part takes the separator after it
			lits[i+1] = ""
		}
	}
	b.WriteString(lits[len(lits)-1])
	return b.String()
}

// Parse reads alias's parts under r's alias template, or reports false when
// the alias does not follow it.
func (r Rules) Parse(alias string) (map[string]string, bool) {
	r = r.WithDefaults()
	if CheckAlias(r.Alias) != nil {
		return nil, false
	}
	re := pattern(r.Alias, map[string][]string{"env": r.Envs, "role": r.Roles})
	m := re.FindStringSubmatch(alias)
	if m == nil && (len(r.Envs) > 0 || len(r.Roles) > 0) {
		// Read it without the lists, so Violation can say which value is off.
		re = pattern(r.Alias, nil)
		m = re.FindStringSubmatch(alias)
	}
	if m == nil {
		return nil, false
	}
	parts := map[string]string{}
	for i, n := range re.SubexpNames() {
		if n != "" && m[i] != "" {
			parts[n] = m[i]
		}
	}
	return parts, true
}

// Violation says why alias does not follow r for a profile whose {code} is
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
// lowest number, from Start, that no name in taken already has with the same
// other parts. A required part with no value is an error; an optional one is
// left out.
func (r Rules) Generate(parts map[string]string, taken []string) (string, error) {
	r = r.WithDefaults()
	if err := CheckAlias(r.Alias); err != nil {
		return "", err
	}
	_, toks := parse(r.Alias)
	width := 2
	for _, t := range toks {
		if t.name == "nn" {
			width = t.width
			continue
		}
		v := parts[t.name]
		if v == "" {
			if t.optional {
				continue
			}
			if t.name == "code" {
				return "", errors.New("the profile has no code to name its servers by")
			}
			return "", fmt.Errorf("naming the server needs its %s (%s)", t.name, r.Alias)
		}
		if !valueRe.MatchString(v) {
			return "", fmt.Errorf("%s %q: use lowercase letters and digits", t.name, v)
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
			if t.name != "nn" && p[t.name] != parts[t.name] {
				same = false
			}
		}
		if n, err := strconv.Atoi(p["nn"]); same && err == nil {
			used[n] = true
		}
	}
	n := r.Start
	for used[n] {
		n++
	}
	fill := map[string]string{"nn": fmt.Sprintf("%0*d", width, n)}
	for _, t := range toks {
		if t.name != "nn" {
			fill[t.name] = parts[t.name]
		}
	}
	return render(r.Alias, fill), nil
}

// ErrNoParts is a key-name template naming a part the host does not have: its
// alias does not follow the alias template, or a required {code} is unknown.
// The caller falls back to the default name.
var ErrNoParts = errors.New("the host lacks a part the key-name template needs")

// RenderKeyName renders r's key-name template for a host. profile and alias
// arrive normalised, code is the profile's {code} ("" for none); the other
// parts come from parsing alias. A required part the host does not have is
// ErrNoParts.
func (r Rules) RenderKeyName(profile, alias, code, algo string) (string, error) {
	r = r.WithDefaults()
	vals := map[string]string{"profile": profile, "alias": alias, "algo": algo, "code": code}
	_, toks := parse(r.KeyName)
	needParts := false
	for _, t := range toks {
		if t.name == "env" || t.name == "role" || t.name == "nn" || (t.name == "code" && code == "") {
			needParts = true
		}
	}
	if needParts {
		if parts, ok := r.Parse(alias); ok {
			for k, v := range parts {
				if vals[k] == "" {
					vals[k] = v
				}
			}
		}
	}
	for _, t := range toks {
		if !t.optional && vals[t.name] == "" {
			return "", ErrNoParts
		}
	}
	return render(r.KeyName, vals), nil
}

// Tokens lists the parts tpl names, sorted, the number as nn.
func Tokens(tpl string) []string {
	_, toks := parse(tpl)
	out := make([]string, 0, len(toks))
	for _, t := range toks {
		out = append(out, t.name)
	}
	sort.Strings(out)
	return out
}

// Required reports whether tpl needs part, rather than naming it optionally
// or not at all.
func Required(tpl, part string) bool {
	_, toks := parse(tpl)
	for _, t := range toks {
		if t.name == part && !t.optional {
			return true
		}
	}
	return false
}
