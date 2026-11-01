package tokens

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// The four credential kinds.
const (
	KindToken   = "token"
	KindBasic   = "basic"
	KindPair    = "pair"
	KindHeaders = "headers"
)

// Credential is one fully resolved entry; its secrets are already read.
type Credential struct {
	Target     string
	Host       string
	Kind       string
	Mechanism  string // composer only: the auth.json section
	Token      string
	Username   string
	Password   string
	Key        string
	Secret     string
	Headers    []string
	Scope      string // npm only: the @org bound to this registry
	AlwaysAuth bool   // npm only
}

func (c Credential) complete() bool {
	switch c.Kind {
	case KindToken:
		return c.Token != ""
	case KindBasic:
		return c.Username != "" && c.Password != ""
	case KindPair:
		return c.Key != "" && c.Secret != ""
	case KindHeaders:
		return len(c.Headers) > 0
	}
	return false
}

// Set is an ordered collection of credentials, at most one per (target,
// host): adding a host again replaces it and moves it to the end. An entry
// whose secret is empty is dropped, never stored half-built.
type Set struct {
	entries  []Credential
	Warnings []string
}

// NewSet returns an empty set.
func NewSet() *Set { return &Set{} }

// Add stores c if its required secrets are present; trailing whitespace a
// secret manager left on a value is trimmed first. It reports whether c was
// kept.
func (s *Set) Add(c Credential) bool {
	c.Token = strings.TrimSpace(c.Token)
	c.Username = strings.TrimSpace(c.Username)
	c.Password = strings.TrimSpace(c.Password)
	c.Key = strings.TrimSpace(c.Key)
	c.Secret = strings.TrimSpace(c.Secret)
	var hs []string
	for _, h := range c.Headers {
		if h != "" {
			hs = append(hs, h)
		}
	}
	c.Headers = hs
	if !c.complete() {
		return false
	}
	s.Remove(c.Target, c.Host)
	s.entries = append(s.entries, c)
	return true
}

// Entries returns the credentials for target, or all of them for "".
func (s *Set) Entries(target string) []Credential {
	var out []Credential
	for _, e := range s.entries {
		if target == "" || e.Target == target {
			out = append(out, e)
		}
	}
	return out
}

// Targets lists the targets that have at least one credential, in order.
func (s *Set) Targets() []string {
	var out []string
	for _, t := range Targets() {
		if len(s.Entries(t)) > 0 {
			out = append(out, t)
		}
	}
	return out
}

// Len counts the credentials for target, or all of them for "".
func (s *Set) Len(target string) int { return len(s.Entries(target)) }

// Remove drops the credential for (target, host) and reports whether one was.
func (s *Set) Remove(target, host string) bool {
	kept := s.entries[:0:0]
	for _, e := range s.entries {
		if !(e.Target == target && e.Host == host) {
			kept = append(kept, e)
		}
	}
	removed := len(kept) != len(s.entries)
	s.entries = kept
	return removed
}

// composerDoc is the auth.json content for the composer entries.
func (s *Set) composerDoc() *object {
	out := newObject()
	for _, e := range s.Entries(Composer) {
		secAny, ok := out.get(e.Mechanism)
		sec, _ := secAny.(*object)
		if !ok || sec == nil {
			sec = newObject()
			out.set(e.Mechanism, sec)
		}
		switch e.Kind {
		case KindToken:
			sec.set(e.Host, e.Token)
		case KindBasic:
			v := newObject()
			v.set("username", e.Username)
			v.set("password", e.Password)
			sec.set(e.Host, v)
		case KindPair:
			v := newObject()
			v.set("consumer-key", e.Key)
			v.set("consumer-secret", e.Secret)
			sec.set(e.Host, v)
		case KindHeaders:
			sec.set(e.Host, append([]string(nil), e.Headers...))
		}
	}
	return out
}

// ComposerJSON is the composer entries as a fresh auth.json document.
func (s *Set) ComposerJSON() string { return encodeIndented(s.composerDoc()) }

// ComposerEnv is the COMPOSER_AUTH export line for the given shell family.
func (s *Set) ComposerEnv(shell string) string {
	return EnvExport("COMPOSER_AUTH", encodeCompact(s.composerDoc()), shell)
}

// NPMRC renders the npm entries as .npmrc lines.
func (s *Set) NPMRC() string {
	var lines []string
	for _, e := range s.Entries(NPM) {
		reg := npmRegistry(e.Host)
		if e.Scope != "" {
			host := strings.TrimRight(e.Host, "/")
			scheme := "https://"
			if strings.Contains(host, "://") {
				scheme = ""
			}
			lines = append(lines, fmt.Sprintf("%s:registry=%s%s/", e.Scope, scheme, host))
		}
		lines = append(lines, reg+"/:_authToken="+e.Token)
		if e.AlwaysAuth {
			lines = append(lines, reg+"/:always-auth=true")
		}
	}
	return strings.Join(lines, "\n")
}

func (s *Set) dockerAuths() *object {
	auths := newObject()
	for _, e := range s.Entries(Docker) {
		v := newObject()
		v.set("auth", base64.StdEncoding.EncodeToString([]byte(e.Username+":"+e.Password)))
		auths.set(e.Host, v)
	}
	return auths
}

// DockerJSON is the docker entries as a fresh config.json document.
func (s *Set) DockerJSON() string {
	doc := newObject()
	doc.set("auths", s.dockerAuths())
	return encodeIndented(doc)
}

// GitCredentials renders the git entries as credential-store lines.
func (s *Set) GitCredentials() string {
	var lines []string
	for _, e := range s.Entries(Git) {
		prefix, bare := "https://", e.Host
		if proto, rest, ok := strings.Cut(e.Host, "://"); ok {
			prefix, bare = proto+"://", rest
		}
		lines = append(lines, prefix+quote(e.Username)+":"+quote(e.Password)+"@"+bare)
	}
	return strings.Join(lines, "\n")
}

// NetrcText renders the netrc entries, one machine per line.
func (s *Set) NetrcText() string {
	var lines []string
	for _, e := range s.Entries(Netrc) {
		lines = append(lines, fmt.Sprintf("machine %s login %s password %s", e.Host, e.Username, e.Password))
	}
	return strings.Join(lines, "\n")
}

// RedactedJSON dumps the entries, secrets masked, for review; only lets
// limit it to some targets (nil for all).
func (s *Set) RedactedJSON(only map[string]bool) string {
	view := newObject()
	for _, e := range s.entries {
		if only != nil && !only[e.Target] {
			continue
		}
		item := newObject()
		item.set("host", e.Host)
		item.set("kind", e.Kind)
		for _, kv := range [][2]string{{"token", e.Token}, {"password", e.Password}, {"secret", e.Secret}} {
			if kv[1] != "" {
				item.set(kv[0], Mask)
			}
		}
		if e.Username != "" {
			item.set("username", e.Username)
		}
		if e.Key != "" {
			item.set("key", Mask)
		}
		if len(e.Headers) > 0 {
			item.set("headers", fmt.Sprintf("%d header(s)", len(e.Headers)))
		}
		if e.Scope != "" {
			item.set("scope", e.Scope)
		}
		list, _ := view.get(e.Target)
		arr, _ := list.([]any)
		view.set(e.Target, append(arr, item))
	}
	return encodeIndented(view)
}

// Mask stands in for a secret anywhere one is shown without being revealed.
const Mask = "********"

// npmRegistry is the .npmrc key for a host: "//host", without scheme or
// trailing slash.
func npmRegistry(host string) string {
	h := strings.TrimRight(host, "/")
	if _, rest, ok := strings.Cut(h, "://"); ok {
		h = rest
	}
	if strings.HasPrefix(h, "//") {
		return h
	}
	return "//" + h
}
