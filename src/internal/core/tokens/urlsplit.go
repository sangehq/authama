package tokens

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
)

// The git credential store is a list of URLs, and the Python tool keyed every
// line by what urllib.parse.urlsplit made of it. A line written or removed by
// one tool must be found by the other, so this is a faithful port of the parts
// of urlsplit the key depends on, including when it raises.

var errBadURL = errors.New("malformed URL")

type urlParts struct {
	netloc string
	path   string
}

// splitURL ports urlsplit's scheme / netloc / path split.
func splitURL(raw string) (urlParts, error) {
	url := strings.TrimLeft(raw, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f ")
	url = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(url)
	if i := strings.IndexByte(url, ':'); i > 0 && validScheme(url[:i]) {
		url = url[i+1:]
	}
	var p urlParts
	if strings.HasPrefix(url, "//") {
		rest := url[2:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			end = len(rest)
		}
		p.netloc, url = rest[:end], rest[end:]
		if strings.Contains(p.netloc, "[") != strings.Contains(p.netloc, "]") {
			return p, errBadURL
		}
		if strings.Contains(p.netloc, "[") {
			if _, err := bracketedHost(p.netloc); err != nil {
				return p, err
			}
		}
	}
	if i := strings.IndexByte(url, '#'); i >= 0 {
		url = url[:i]
	}
	if i := strings.IndexByte(url, '?'); i >= 0 {
		url = url[:i]
	}
	p.path = url
	return p, nil
}

func validScheme(s string) bool {
	if s == "" || !isASCIILetter(rune(s[0])) {
		return false
	}
	for _, r := range s {
		if !isASCIILetter(r) && !(r >= '0' && r <= '9') && r != '+' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

func isASCIILetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

func bracketedHost(netloc string) (string, error) {
	hostinfo := netloc[strings.LastIndexByte(netloc, '@')+1:]
	_, after, _ := strings.Cut(hostinfo, "[")
	host, _, _ := strings.Cut(after, "]")
	if strings.HasPrefix(host, "v") {
		return host, nil // IPvFuture; urlsplit only pattern-checks it
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Is6() {
		return "", errBadURL
	}
	return host, nil
}

func (p urlParts) userinfo() (string, bool) {
	i := strings.LastIndexByte(p.netloc, '@')
	if i < 0 {
		return "", false
	}
	return p.netloc[:i], true
}

func (p urlParts) username() string {
	ui, ok := p.userinfo()
	if !ok {
		return ""
	}
	u, _, _ := strings.Cut(ui, ":")
	return u
}

// password returns the password and whether the URL had one at all.
func (p urlParts) password() (string, bool) {
	ui, ok := p.userinfo()
	if !ok {
		return "", false
	}
	_, pw, has := strings.Cut(ui, ":")
	return pw, has
}

func (p urlParts) hostinfo() string { return p.netloc[strings.LastIndexByte(p.netloc, '@')+1:] }

// hostname is urlsplit().hostname: lower-cased, brackets removed, "" for none.
func (p urlParts) hostname() string {
	hi := p.hostinfo()
	var host string
	if strings.Contains(hi, "[") {
		_, after, _ := strings.Cut(hi, "[")
		host, _, _ = strings.Cut(after, "]")
	} else {
		host, _, _ = strings.Cut(hi, ":")
	}
	h, zone, hasZone := strings.Cut(host, "%")
	if hasZone {
		return strings.ToLower(h) + "%" + zone
	}
	return strings.ToLower(host)
}

// port is urlsplit().port: 0 when absent, an error when not a number in range.
func (p urlParts) port() (int, error) {
	hi := p.hostinfo()
	var port string
	if strings.Contains(hi, "[") {
		_, after, _ := strings.Cut(hi, "]")
		_, port, _ = strings.Cut(after, ":")
	} else {
		_, port, _ = strings.Cut(hi, ":")
	}
	if port == "" {
		return 0, nil
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return 0, errBadURL
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n > 65535 {
		return 0, errBadURL
	}
	return n, nil
}

// quote is urllib.parse.quote(value, safe=""): only unreserved characters
// survive, everything else is %XX-escaped from its UTF-8 bytes.
func quote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isASCIILetter(rune(c)) || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '-' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(c)|0x100, 16)[1:]))
	}
	return b.String()
}

// unquote is urllib.parse.unquote: valid %XX escapes become bytes, invalid
// ones stay literal, and bytes that are not UTF-8 become U+FFFD.
func unquote(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var raw []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			v, _ := strconv.ParseUint(s[i+1:i+3], 16, 8)
			raw = append(raw, byte(v))
			i += 2
			continue
		}
		raw = append(raw, s[i])
	}
	return toValidUTF8(raw)
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// GitKey is the canonical key of a git credential line or a bare host: the
// lower-cased hostname, an explicit port, and any path, without scheme,
// credentials or trailing slash, so "dev.azure.com/org" and "git.acme:8443"
// round-trip for listing and removal.
func GitKey(value string) string {
	v := value
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}
	p, err := splitURL(v)
	var port int
	if err == nil {
		port, err = p.port()
	}
	if err != nil {
		rest := strings.SplitN(v, "://", 2)[1]
		rest = rest[strings.LastIndex(rest, "@")+1:]
		netloc, path, slash := strings.Cut(rest, "/")
		key := strings.ToLower(netloc)
		if slash {
			key += "/" + path
		}
		return strings.TrimRight(key, "/")
	}
	host := p.hostname()
	if host != "" && port != 0 {
		host += ":" + strconv.Itoa(port)
	}
	return strings.TrimRight(host+p.path, "/")
}
