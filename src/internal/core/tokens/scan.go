package tokens

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Installed is one credential found in an auth file on disk. Secret is the
// value itself and is only filled in when a caller asks to reveal it.
type Installed struct {
	Target        string `json:"target"`
	Host          string `json:"host"`
	Kind          string `json:"kind"` // the composer section, or token / basic
	Username      string `json:"username"`
	SecretPresent bool   `json:"secret_present"`
	Secret        string `json:"secret"`
	Description   string `json:"description"`
	File          string `json:"file"`
}

func mk(target, host, kind, user string, present bool, secret string) Installed {
	return Installed{Target: target, Host: host, Kind: kind, Username: user, SecretPresent: present, Secret: secret}
}

// Scan reads the credentials out of one target's file content. It never
// fails: content it cannot parse holds no credentials it can list.
func Scan(target, text string) []Installed {
	var out []Installed
	switch target {
	case Composer:
		out = scanComposer(text)
	case NPM:
		out = scanNPM(text)
	case Docker:
		out = scanDocker(text)
	case Git:
		out = scanGit(text)
	case Netrc:
		out = scanNetrc(text)
	}
	for i := range out {
		out[i].Description = Describe(out[i].Target, out[i].Host)
	}
	return out
}

func scanComposer(text string) []Installed {
	var out []Installed
	doc, err := decodeOrdered(text)
	if err != nil {
		return nil
	}
	root, ok := doc.(*object)
	if !ok {
		return nil
	}
	for _, section := range root.keys {
		hosts, ok := root.vals[section].(*object)
		if !ok {
			continue
		}
		for _, host := range hosts.keys {
			switch v := hosts.vals[host].(type) {
			case string:
				out = append(out, mk(Composer, host, section, "", v != "", v))
			case *object:
				user := firstTruthy(v, "username", "consumer-key")
				secret := firstTruthy(v, "password", "consumer-secret")
				out = append(out, mk(Composer, host, section, user, secret != "", secret))
			case []any:
				parts := make([]string, len(v))
				for i, h := range v {
					parts[i] = pyStr(h)
				}
				out = append(out, mk(Composer, host, section, "", len(v) > 0, strings.Join(parts, "; ")))
			}
		}
	}
	return out
}

// firstTruthy is `o.get(a) or o.get(b) or ""`, stringified as Python would.
func firstTruthy(o *object, keys ...string) string {
	for _, k := range keys {
		if v, ok := o.get(k); ok && truthy(v) {
			return pyStr(v)
		}
	}
	return ""
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return t != ""
	case bool:
		return t
	case json.Number:
		f, err := t.Float64()
		return err != nil || f != 0
	case *object:
		return len(t.keys) > 0
	case []any:
		return len(t) > 0
	}
	return true
}

// pyStr is str(v) for the JSON values a credential field can hold.
func pyStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "True"
		}
		return "False"
	case nil:
		return "None"
	case json.Number:
		return t.String()
	}
	return encodeCompact(v)
}

var (
	npmToken    = regexp.MustCompile(`^//(.+?)/?:_authToken=(.+)`)
	npmAuth     = regexp.MustCompile(`^//(.+?)/?:_auth=(.+)`)
	npmUsername = regexp.MustCompile(`^//(.+?)/?:username=(.+)`)
	npmPassword = regexp.MustCompile(`^//(.+?)/?:_password=(.+)`)
)

func scanNPM(text string) []Installed {
	var out []Installed
	seen := map[string]bool{}
	type split struct{ user, pass string }
	splits := map[string]*split{}
	var order []string
	get := func(h string) *split {
		if splits[h] == nil {
			splits[h] = &split{}
			order = append(order, h)
		}
		return splits[h]
	}
	for _, line := range pySplitLines(text) {
		st := pyStrip(line)
		if m := npmToken.FindStringSubmatch(st); m != nil {
			tok := pyStrip(m[2])
			out = append(out, mk(NPM, m[1], KindToken, "", tok != "", tok))
			seen[m[1]] = true
			continue
		}
		if m := npmAuth.FindStringSubmatch(st); m != nil {
			raw := pyStrip(m[2])
			user, pw := "", ""
			if dec, ok := b64decode(raw); ok {
				user, pw, _ = strings.Cut(dec, ":")
			}
			out = append(out, mk(NPM, m[1], KindBasic, user, raw != "", pw))
			seen[m[1]] = true
			continue
		}
		if m := npmUsername.FindStringSubmatch(st); m != nil {
			get(m[1]).user = pyStrip(m[2])
			continue
		}
		if m := npmPassword.FindStringSubmatch(st); m != nil {
			get(m[1]).pass = pyStrip(m[2])
		}
	}
	for _, h := range order {
		if seen[h] {
			continue
		}
		sp := splits[h]
		pw := ""
		if sp.pass != "" {
			pw, _ = b64decode(sp.pass)
		}
		out = append(out, mk(NPM, h, KindBasic, sp.user, sp.pass != "", pw))
	}
	return out
}

// b64decode is base64.b64decode(...).decode("utf-8", "replace"): characters
// outside the alphabet are discarded, and bad padding fails.
func b64decode(s string) (string, bool) {
	var clean strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			clean.WriteRune(r)
		}
	}
	c := clean.String()
	if i := strings.IndexByte(c, '='); i >= 0 {
		// Python stops at the first complete padded quantum.
		c = strings.TrimRight(c[:i], "=") + c[i:]
		end := i
		for end < len(c) && c[end] == '=' {
			end++
		}
		c = c[:end]
	}
	b, err := base64.StdEncoding.DecodeString(c)
	if err != nil {
		return "", false
	}
	return toValidUTF8(b), true
}

// toValidUTF8 is bytes.decode("utf-8", "replace").
func toValidUTF8(b []byte) string {
	var sb strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		sb.WriteRune(r)
		b = b[n:]
	}
	return sb.String()
}

func scanDocker(text string) []Installed {
	var out []Installed
	doc, err := decodeOrdered(text)
	if err != nil {
		return nil
	}
	root, ok := doc.(*object)
	if !ok {
		return nil
	}
	authsAny, _ := root.get("auths")
	auths, ok := authsAny.(*object)
	if !ok {
		return nil
	}
	for _, host := range auths.keys {
		user, secret := "", ""
		var auth, idtoken any
		if v, ok := auths.vals[host].(*object); ok {
			auth, _ = v.get("auth")
			idtoken, _ = v.get("identitytoken")
		}
		if truthy(auth) {
			if s, ok := auth.(string); ok {
				if dec, ok := b64decode(s); ok {
					user, secret, _ = strings.Cut(dec, ":")
				}
			}
		} else if truthy(idtoken) {
			secret = pyStr(idtoken)
		}
		out = append(out, mk(Docker, host, KindBasic, user, truthy(auth) || truthy(idtoken), secret))
	}
	return out
}

func scanGit(text string) []Installed {
	var out []Installed
	for _, line := range pySplitLines(text) {
		st := pyStrip(line)
		if st == "" || strings.HasPrefix(st, "#") {
			continue
		}
		u := st
		if !strings.Contains(u, "://") {
			u = "https://" + u
		}
		p, err := splitURL(u)
		var port int
		if err == nil {
			port, err = p.port()
		}
		if err != nil {
			out = append(out, mk(Git, GitKey(st), KindBasic, "", false, ""))
			continue
		}
		host := p.hostname()
		if host == "" {
			continue
		}
		if port != 0 {
			host += ":" + itoa(port)
		}
		host = strings.TrimRight(host+p.path, "/")
		pw, hasPW := p.password()
		out = append(out, mk(Git, host, KindBasic, unquote(p.username()), hasPW && pw != "", unquote(pw)))
	}
	return out
}

type netrcBlock struct {
	kind string // machine | default
	name string
	kv   [][2]string
}

// parseNetrc reads .netrc into ordered blocks; several machines on one line
// parse correctly and macdef bodies are skipped (best effort).
func parseNetrc(text string) []netrcBlock {
	toks := pyFields(text)
	starts := map[string]bool{"machine": true, "default": true, "macdef": true}
	var out []netrcBlock
	i, n := 0, len(toks)
	for i < n {
		t := toks[i]
		if t != "machine" && t != "default" {
			i++
			continue
		}
		var name string
		switch {
		case t == "machine" && i+1 < n:
			name, i = toks[i+1], i+2
		case t == "default":
			i++
		default:
			return out
		}
		b := netrcBlock{kind: t, name: name}
		for i < n && !starts[toks[i]] {
			hasValue := i+1 < n && !starts[toks[i+1]]
			if (toks[i] == "login" || toks[i] == "password" || toks[i] == "account") && hasValue {
				b.kv = append(b.kv, [2]string{toks[i], toks[i+1]})
				i += 2
			} else {
				i++
			}
		}
		out = append(out, b)
	}
	return out
}

func scanNetrc(text string) []Installed {
	var out []Installed
	for _, b := range parseNetrc(text) {
		if b.kind != "machine" {
			continue
		}
		d := map[string]string{}
		has := false
		for _, kv := range b.kv {
			d[kv[0]] = kv[1]
			if kv[0] == "password" {
				has = true
			}
		}
		out = append(out, mk(Netrc, b.name, KindBasic, d["login"], has, d["password"]))
	}
	return out
}

// Purge removes host from one target's file content. The caller checks the
// host is present first (Scan) and appends the final newline.
func Purge(target, host, text string) string {
	switch target {
	case Composer:
		doc, err := decodeOrdered(text)
		if err != nil {
			return text
		}
		root, ok := doc.(*object)
		if !ok {
			return text
		}
		for _, section := range append([]string(nil), root.keys...) {
			if sec, ok := root.vals[section].(*object); ok {
				sec.del(host)
				if len(sec.keys) == 0 {
					root.del(section)
				}
			}
		}
		return encodeIndented(root)
	case Docker:
		doc, err := decodeOrdered(text)
		if err != nil {
			return text
		}
		if root, ok := doc.(*object); ok {
			if auths, ok := root.vals["auths"].(*object); ok {
				auths.del(host)
			}
		}
		return encodeIndented(doc)
	case NPM:
		reg := npmRegistry(host)
		var keep []string
		for _, ln := range pySplitLines(text) {
			st := pyStrip(ln)
			if strings.HasPrefix(st, reg+"/:") || strings.HasPrefix(st, reg+":") {
				continue
			}
			if strings.HasPrefix(st, "@") && strings.Contains(st, ":registry=") {
				url := pyStrip(strings.SplitN(st, ":registry=", 2)[1])
				if npmRegistry(url) == reg {
					continue
				}
			}
			keep = append(keep, ln)
		}
		return strings.Join(keep, "\n")
	case Git:
		key := GitKey(host)
		var keep []string
		for _, ln := range pySplitLines(text) {
			if pyStrip(ln) == "" || GitKey(pyStrip(ln)) != key {
				keep = append(keep, ln)
			}
		}
		return strings.Join(keep, "\n")
	case Netrc:
		return purgeNetrc(host, text)
	}
	return text
}

func purgeNetrc(host, text string) string {
	lines := pySplitLines(text)
	hasComment := false
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimLeftFunc(ln, pySpace), "#") {
			hasComment = true
		}
	}
	if strings.Contains(text, "macdef") || hasComment {
		// macdef bodies and comments cannot survive re-serialising tokens,
		// so drop whole lines instead.
		var keep []string
		for _, ln := range lines {
			f := pyFields(ln)
			if len(f) >= 2 && f[0] == "machine" && f[1] == host {
				continue
			}
			keep = append(keep, ln)
		}
		return strings.Join(keep, "\n")
	}
	var out []string
	for _, b := range parseNetrc(text) {
		if b.kind == "machine" && b.name == host {
			continue
		}
		parts := []string{"default"}
		if b.kind == "machine" {
			parts = []string{"machine", b.name}
		}
		for _, kv := range b.kv {
			parts = append(parts, kv[0], kv[1])
		}
		out = append(out, strings.Join(parts, " "))
	}
	return strings.Join(out, "\n")
}

// CanonicalHost is the host key Scan reports for a user-typed host: git
// accepts a bare host, a full URL or a trailing slash.
func CanonicalHost(target, host string) string {
	if target == Git {
		return GitKey(host)
	}
	return host
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// DockerDelegation reads credsStore and credHelpers from a docker config;
// both are empty when the text is not a JSON object.
func DockerDelegation(text string) (store string, helpers map[string]string) {
	helpers = map[string]string{}
	doc, err := decodeOrdered(text)
	if err != nil {
		return "", helpers
	}
	root, ok := doc.(*object)
	if !ok {
		return "", helpers
	}
	store, _ = root.vals["credsStore"].(string)
	if h, ok := root.vals["credHelpers"].(*object); ok {
		for _, k := range h.keys {
			if v, ok := h.vals[k].(string); ok && v != "" {
				helpers[k] = v
			}
		}
	}
	return store, helpers
}
