package tokens

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The tools whose files are XML or YAML: Maven, Yarn Berry, Helm (classic
// repositories and OCI registries) and the GitHub CLI. Each edits one entry and
// leaves every other line, key and comment of the file where it was: XML by
// splicing the text around one <server> element, YAML through the node tree,
// which keeps key order and comments.
const (
	Maven   = "maven"
	Yarn    = "yarn"
	Helm    = "helm"
	HelmOCI = "helm-oci"
	GH      = "gh"
)

func init() {
	formats[Maven] = mavenSettings{}
	formats[Yarn] = yarnrc{}
	formats[Helm] = helmRepos{}
	formats[HelmOCI] = helmRegistry{}
	formats[GH] = ghHosts{}
	targetRole[Maven] = "Maven repository login"
	targetRole[Yarn] = "Yarn registry token"
	targetRole[Helm] = "Helm chart repository login"
	targetRole[HelmOCI] = "Helm OCI registry login"
	targetRole[GH] = "GitHub CLI login"
}

// helmConfigHome is HELM_CONFIG_HOME, else where Helm puts its config on each
// platform (`helm env`).
func helmConfigHome(h Host) string {
	if v := h.env("HELM_CONFIG_HOME"); v != "" {
		return h.expandUser(v)
	}
	switch {
	case h.Windows:
		if a := h.env("APPDATA"); a != "" {
			return filepath.Join(h.expandUser(a), "helm")
		}
		return filepath.Join(h.Home, "AppData", "Roaming", "helm")
	case h.MacOS:
		return filepath.Join(h.Home, "Library", "Preferences", "helm")
	}
	if x := h.env("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(h.expandUser(x), "helm")
	}
	return filepath.Join(h.Home, ".config", "helm")
}

// --- Maven: ~/.m2/settings.xml --------------------------------------------------

// mavenSettings is ~/.m2/settings.xml. A host is a <server> id, which Maven
// matches exactly against a repository's or mirror's id in the build.
type mavenSettings struct{}

var mavenID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (mavenSettings) kind() string { return KindBasic }

func (mavenSettings) path(h Host) string { return filepath.Join(h.Home, ".m2", "settings.xml") }

func (mavenSettings) check(c Credential) error {
	if !mavenID.MatchString(c.Host) {
		return errors.New("a Maven server id is letters, digits and . _ -, as the repository's <id> in the build")
	}
	return nil
}

// xmlServer is one <server> element: its id, credentials and byte span.
type xmlServer struct {
	id, user, pass string
	start, end     int
}

// mavenLayout reads the <server> elements of settings.xml with their byte
// spans, and where </servers> and </settings> start, without changing a byte:
// the decoder only finds offsets, and edits splice the original text.
func mavenLayout(text string) (servers []xmlServer, serversEnd, settingsEnd int) {
	serversEnd, settingsEnd = -1, -1
	d := xml.NewDecoder(strings.NewReader(text))
	var path []string
	var cur *xmlServer
	var field string
	for {
		off := int(d.InputOffset())
		tok, err := d.Token()
		if err != nil {
			return servers, serversEnd, settingsEnd
		}
		switch t := tok.(type) {
		case xml.StartElement:
			path = append(path, t.Name.Local)
			if strings.Join(path, "/") == "settings/servers/server" {
				cur = &xmlServer{start: off}
			} else if cur != nil && len(path) == 4 {
				field = t.Name.Local
			}
		case xml.CharData:
			if cur != nil && len(path) == 4 {
				v := strings.TrimSpace(string(t))
				switch field {
				case "id":
					cur.id += v
				case "username":
					cur.user += v
				case "password":
					cur.pass += v
				}
			}
		case xml.EndElement:
			switch strings.Join(path, "/") {
			case "settings/servers/server":
				if cur != nil {
					cur.end = int(d.InputOffset())
					servers = append(servers, *cur)
					cur = nil
				}
			case "settings/servers":
				serversEnd = off
			case "settings":
				settingsEnd = off
			}
			field = ""
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}
}

func mavenUnescape(s string) string {
	return strings.NewReplacer(`\{`, "{", `\}`, "}").Replace(s)
}

// mavenText escapes a value for settings.xml: XML's own characters, and the
// braces Maven reads as an encrypted value ({...}) unless backslashed.
func mavenText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return strings.NewReplacer("{", `\{`, "}", `\}`).Replace(b.String())
}

func (m mavenSettings) scan(text string) []Installed {
	servers, _, _ := mavenLayout(text)
	var out []Installed
	for _, s := range servers {
		if s.id == "" {
			continue
		}
		out = append(out, mk(Maven, s.id, KindBasic, mavenUnescape(s.user), s.pass != "", mavenUnescape(s.pass)))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

// cutSpan removes text[start:end] and, when the element was alone on its
// lines, the indentation before it and the line ending after it.
func cutSpan(text string, start, end int) string {
	ls := start
	for ls > 0 && (text[ls-1] == ' ' || text[ls-1] == '\t') {
		ls--
	}
	if ls == 0 || text[ls-1] == '\n' {
		le := end
		for le < len(text) && (text[le] == ' ' || text[le] == '\t' || text[le] == '\r') {
			le++
		}
		if le < len(text) && text[le] == '\n' {
			return text[:ls] + text[le+1:]
		}
		if le == len(text) {
			return text[:ls]
		}
	}
	return text[:start] + text[end:]
}

func (m mavenSettings) purge(host, text string) string {
	for {
		servers, _, _ := mavenLayout(text)
		i := -1
		for j, s := range servers {
			if s.id == host {
				i = j
			}
		}
		if i < 0 {
			return text
		}
		text = cutSpan(text, servers[i].start, servers[i].end)
	}
}

func (m mavenSettings) merge(entries []Credential, existing string) string {
	text := existing
	for _, e := range entries {
		text = m.purge(e.Host, text)
	}
	if len(entries) == 0 {
		return text
	}
	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "    <server>\n      <id>%s</id>\n      <username>%s</username>\n      <password>%s</password>\n    </server>\n",
			mavenText(e.Host), mavenText(e.Username), mavenText(e.Password))
	}
	blocks := b.String()
	if strings.TrimSpace(text) == "" {
		return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
			"<settings xmlns=\"http://maven.apache.org/SETTINGS/1.0.0\">\n  <servers>\n" + blocks + "  </servers>\n</settings>\n"
	}
	_, serversEnd, settingsEnd := mavenLayout(text)
	switch {
	case serversEnd >= 0:
		return insertAtLine(text, serversEnd, blocks)
	case settingsEnd >= 0:
		return insertAtLine(text, settingsEnd, "  <servers>\n"+blocks+"  </servers>\n")
	}
	// Not a settings document authama can find its way in: left as it was.
	return existing
}

// insertAtLine puts block before the closing tag at pos, at the start of its
// line when only indentation precedes the tag there.
func insertAtLine(text string, pos int, block string) string {
	ls := pos
	for ls > 0 && (text[ls-1] == ' ' || text[ls-1] == '\t') {
		ls--
	}
	if ls == 0 || text[ls-1] == '\n' {
		return text[:ls] + block + text[ls:]
	}
	return text[:pos] + "\n" + block + text[pos:]
}

// --- YAML helpers ------------------------------------------------------------------

// yamlDoc parses text into its root mapping. Empty text is a fresh document;
// anything else that is not a mapping is not a file authama will edit.
func yamlDoc(text string) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if strings.TrimSpace(text) != "" {
		if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
			return nil, nil, err
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil, errors.New("not a YAML mapping")
	}
	return &doc, doc.Content[0], nil
}

func yamlText(doc *yaml.Node) string {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return ""
	}
	_ = enc.Close()
	return b.String()
}

// yamlGet is the value under key in mapping m, or nil.
func yamlGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func yamlStr(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v, Style: yaml.DoubleQuotedStyle}
}

// yamlSet puts val under key in mapping m, in place when the key is there.
func yamlSet(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
}

func yamlDel(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// yamlMap is the mapping under key in m, created when absent.
func yamlMap(m *yaml.Node, key string) *yaml.Node {
	if v := yamlGet(m, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	yamlSet(m, key, v)
	return v
}

func yamlScalar(m *yaml.Node, key string) string {
	if v := yamlGet(m, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// --- Yarn Berry: ~/.yarnrc.yml ---------------------------------------------------

// yarnrc is the home ~/.yarnrc.yml. A host is a registry ("npm.example.com"),
// written under npmRegistries as "//npm.example.com", the form Yarn falls back
// to for any scheme.
type yarnrc struct{}

func (yarnrc) kind() string { return KindToken }

func (yarnrc) path(h Host) string { return filepath.Join(h.Home, ".yarnrc.yml") }

func (yarnrc) check(c Credential) error {
	if hasSpaceOrControl(c.Token) {
		return errors.New("a Yarn auth token cannot contain whitespace")
	}
	return nil
}

// yarnKey is a registry's npmRegistries key: "//host[/path]", no trailing
// slash, which Yarn never strips from a key.
func yarnKey(host string) string {
	h := strings.TrimPrefix(strings.TrimPrefix(host, "https:"), "http:")
	return "//" + strings.TrimSuffix(strings.TrimPrefix(h, "//"), "/")
}

func yarnHost(key string) string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(key, "https:"), "http:"), "//")
}

func (yarnrc) scan(text string) []Installed {
	_, root, err := yamlDoc(text)
	if err != nil {
		return nil
	}
	regs := yamlGet(root, "npmRegistries")
	if regs == nil || regs.Kind != yaml.MappingNode {
		return nil
	}
	var out []Installed
	for i := 0; i+1 < len(regs.Content); i += 2 {
		v := regs.Content[i+1]
		tok := yamlScalar(v, "npmAuthToken")
		ident := yamlScalar(v, "npmAuthIdent")
		if tok == "" && ident == "" {
			continue
		}
		user, secret := "", tok
		if tok == "" {
			user, secret, _ = strings.Cut(ident, ":")
		}
		out = append(out, mk(Yarn, yarnHost(regs.Content[i].Value), KindToken, user, true, secret))
	}
	return out
}

// yarnFind is the npmRegistries key that names host, in any of the spellings
// Yarn matches ("//h", "https://h"), or "".
func yarnFind(regs *yaml.Node, host string) string {
	want := yarnKey(host)
	for i := 0; i+1 < len(regs.Content); i += 2 {
		if yarnKey(regs.Content[i].Value) == want {
			return regs.Content[i].Value
		}
	}
	return ""
}

func (y yarnrc) purge(host, text string) string {
	doc, root, err := yamlDoc(text)
	if err != nil {
		return text
	}
	regs := yamlGet(root, "npmRegistries")
	if regs == nil || regs.Kind != yaml.MappingNode {
		return text
	}
	key := yarnFind(regs, host)
	if key == "" {
		return text
	}
	reg := yamlGet(regs, key)
	for _, k := range []string{"npmAuthToken", "npmAuthIdent", "npmAlwaysAuth"} {
		yamlDel(reg, k)
	}
	if reg == nil || reg.Kind != yaml.MappingNode || len(reg.Content) == 0 {
		yamlDel(regs, key)
	}
	if len(regs.Content) == 0 {
		yamlDel(root, "npmRegistries")
	}
	return yamlText(doc)
}

func (y yarnrc) merge(entries []Credential, existing string) string {
	if len(entries) == 0 {
		return existing
	}
	doc, root, err := yamlDoc(existing)
	if err != nil {
		return existing
	}
	regs := yamlMap(root, "npmRegistries")
	for _, e := range entries {
		key := yarnFind(regs, e.Host)
		if key == "" {
			// Quoted, as Yarn's own documentation writes these keys.
			key = yarnKey(e.Host)
			regs.Content = append(regs.Content, yamlStr(key), &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
		}
		reg := yamlMap(regs, key)
		yamlDel(reg, "npmAuthIdent")
		yamlSet(reg, "npmAuthToken", yamlStr(e.Token))
		yamlSet(reg, "npmAlwaysAuth", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
	return yamlText(doc)
}

// --- Helm: repositories.yaml (classic chart repositories) --------------------------

// helmRepos is Helm's repositories.yaml. A host is the repository's URL
// ("https://charts.example.com/stable"); the entry `helm repo add` made for
// it gets the user name and password, and one is added under a name derived
// from the URL when there is none.
type helmRepos struct{}

func (helmRepos) kind() string { return KindBasic }

func (helmRepos) path(h Host) string {
	if v := h.env("HELM_REPOSITORY_CONFIG"); v != "" {
		return h.expandUser(v)
	}
	return filepath.Join(helmConfigHome(h), "repositories.yaml")
}

func (helmRepos) check(c Credential) error {
	u, err := url.Parse(c.Host)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("a Helm repository is its URL, such as https://charts.example.com/stable")
	}
	return nil
}

func sameURL(a, b string) bool { return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/") }

var helmNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// helmName is a repository name for url that no other entry has.
func helmName(list *yaml.Node, raw string) string {
	u, _ := url.Parse(raw)
	base := helmNameUnsafe.ReplaceAllString(strings.Trim(u.Host+u.Path, "/"), "-")
	name := base
	for n := 2; ; n++ {
		taken := false
		for _, it := range list.Content {
			if yamlScalar(it, "name") == name {
				taken = true
			}
		}
		if !taken {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, n)
	}
}

func (helmRepos) scan(text string) []Installed {
	_, root, err := yamlDoc(text)
	if err != nil {
		return nil
	}
	list := yamlGet(root, "repositories")
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	var out []Installed
	for _, it := range list.Content {
		user, pass := yamlScalar(it, "username"), yamlScalar(it, "password")
		if user == "" && pass == "" {
			continue
		}
		out = append(out, mk(Helm, yamlScalar(it, "url"), KindBasic, user, pass != "", pass))
	}
	return out
}

func (helmRepos) purge(host, text string) string {
	doc, root, err := yamlDoc(text)
	if err != nil {
		return text
	}
	list := yamlGet(root, "repositories")
	if list == nil || list.Kind != yaml.SequenceNode {
		return text
	}
	changed := false
	for _, it := range list.Content {
		if sameURL(yamlScalar(it, "url"), host) {
			yamlDel(it, "username")
			yamlDel(it, "password")
			changed = true
		}
	}
	if !changed {
		return text
	}
	return yamlText(doc)
}

func (helmRepos) merge(entries []Credential, existing string) string {
	if len(entries) == 0 {
		return existing
	}
	doc, root, err := yamlDoc(existing)
	if err != nil {
		return existing
	}
	if yamlGet(root, "apiVersion") == nil {
		yamlSet(root, "apiVersion", yamlStr(""))
	}
	list := yamlGet(root, "repositories")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		yamlSet(root, "repositories", list)
	}
	for _, e := range entries {
		var it *yaml.Node
		for _, cand := range list.Content {
			if sameURL(yamlScalar(cand, "url"), e.Host) {
				it = cand
			}
		}
		if it == nil {
			it = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			yamlSet(it, "name", yamlStr(helmName(list, e.Host)))
			yamlSet(it, "url", yamlStr(e.Host))
			list.Content = append(list.Content, it)
		}
		yamlSet(it, "username", yamlStr(e.Username))
		yamlSet(it, "password", yamlStr(e.Password))
	}
	return yamlText(doc)
}

// --- Helm: registry/config.json (OCI registries) ----------------------------------

// helmRegistry is Helm's OCI registry credentials file, in Docker's format:
// {"auths": {"ghcr.io": {"auth": base64(user:pass)}}}. A host is the
// registry's host, as `helm registry login` takes it. A credential store
// named in the file (credsStore, credHelpers) takes precedence over it, as
// it does for Docker.
type helmRegistry struct{}

func (helmRegistry) kind() string { return KindBasic }

func (helmRegistry) path(h Host) string {
	if v := h.env("HELM_REGISTRY_CONFIG"); v != "" {
		return h.expandUser(v)
	}
	return filepath.Join(helmConfigHome(h), "registry", "config.json")
}

func (helmRegistry) check(c Credential) error {
	if strings.Contains(c.Host, "/") {
		return errors.New("a Helm OCI registry is its host, such as ghcr.io, without oci:// or a path")
	}
	return nil
}

func (helmRegistry) scan(text string) []Installed {
	out := scanDocker(text)
	for i := range out {
		out[i].Target = HelmOCI
	}
	return out
}

func (helmRegistry) purge(host, text string) string {
	doc, err := decodeOrdered(text)
	if err != nil {
		return text
	}
	root, ok := doc.(*object)
	if !ok {
		return text
	}
	if auths, ok := root.vals["auths"].(*object); ok {
		auths.del(host)
	}
	return encodeIndented(root)
}

func (helmRegistry) merge(entries []Credential, existing string) string {
	if len(entries) == 0 {
		return existing
	}
	base := newObject()
	if strings.TrimSpace(existing) != "" {
		doc, err := decodeOrdered(existing)
		o, ok := doc.(*object)
		if err != nil || !ok {
			return existing
		}
		base = o.clone()
	}
	auths, _ := base.vals["auths"].(*object)
	if auths == nil {
		auths = newObject()
	} else {
		auths = auths.clone()
	}
	for _, e := range entries {
		v := newObject()
		v.set("auth", base64.StdEncoding.EncodeToString([]byte(e.Username+":"+e.Password)))
		auths.set(e.Host, v)
	}
	base.set("auths", auths)
	return encodeIndented(base)
}

// --- GitHub CLI: hosts.yml -----------------------------------------------------------

// ghHosts is the GitHub CLI's hosts.yml. A host is github.com or an
// Enterprise Server host; the user name is the account's login and the
// password its token. The token is written where gh reads it when it was not
// put in the keyring (--insecure-storage): users.<login>.oauth_token, and the
// host's own oauth_token and user, so the account is the active one and a
// later `gh auth switch` keeps it. GH_TOKEN and GH_ENTERPRISE_TOKEN still win.
type ghHosts struct{}

func (ghHosts) kind() string { return KindBasic }

func (ghHosts) path(h Host) string {
	if v := h.env("GH_CONFIG_DIR"); v != "" {
		return filepath.Join(h.expandUser(v), "hosts.yml")
	}
	if x := h.env("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(h.expandUser(x), "gh", "hosts.yml")
	}
	if h.Windows {
		if a := h.env("APPDATA"); a != "" {
			return filepath.Join(h.expandUser(a), "GitHub CLI", "hosts.yml")
		}
	}
	return filepath.Join(h.Home, ".config", "gh", "hosts.yml")
}

func (ghHosts) check(c Credential) error {
	if strings.Contains(c.Host, "/") || strings.Contains(c.Host, ":") {
		return errors.New("a GitHub CLI host is a host name, such as github.com")
	}
	if c.Username == "" || hasSpaceOrControl(c.Username) {
		return errors.New("the GitHub CLI needs the account's login as the user name")
	}
	if hasSpaceOrControl(c.Password) {
		return errors.New("a GitHub token cannot contain whitespace")
	}
	return nil
}

func (ghHosts) scan(text string) []Installed {
	_, root, err := yamlDoc(text)
	if err != nil {
		return nil
	}
	var out []Installed
	for i := 0; i+1 < len(root.Content); i += 2 {
		hv := root.Content[i+1]
		user := yamlScalar(hv, "user")
		tok := yamlScalar(hv, "oauth_token")
		if tok == "" && user != "" {
			tok = yamlScalar(yamlGet(yamlGet(hv, "users"), user), "oauth_token")
		}
		if tok == "" {
			continue
		}
		out = append(out, mk(GH, root.Content[i].Value, KindBasic, user, true, tok))
	}
	return out
}

func (ghHosts) purge(host, text string) string {
	doc, root, err := yamlDoc(text)
	if err != nil {
		return text
	}
	hv := yamlGet(root, host)
	if hv == nil || hv.Kind != yaml.MappingNode {
		return text
	}
	user := yamlScalar(hv, "user")
	yamlDel(hv, "oauth_token")
	if u := yamlGet(yamlGet(hv, "users"), user); u != nil {
		yamlDel(u, "oauth_token")
	}
	return yamlText(doc)
}

func (ghHosts) merge(entries []Credential, existing string) string {
	if len(entries) == 0 {
		return existing
	}
	doc, root, err := yamlDoc(existing)
	if err != nil {
		return existing
	}
	for _, e := range entries {
		fresh := yamlGet(root, e.Host) == nil
		hv := yamlMap(root, e.Host)
		users := yamlMap(hv, "users")
		u := yamlMap(users, e.Username)
		yamlSet(u, "oauth_token", yamlStr(e.Password))
		yamlSet(hv, "oauth_token", yamlStr(e.Password))
		yamlSet(hv, "user", yamlStr(e.Username))
		if fresh || yamlGet(hv, "git_protocol") == nil {
			yamlSet(hv, "git_protocol", yamlStr("https"))
		}
	}
	return yamlText(doc)
}
