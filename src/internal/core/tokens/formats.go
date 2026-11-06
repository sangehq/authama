package tokens

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// The tools added after the five authama ported from auth-token-manager. Each
// is a fileFormat, so the dispatch in TargetPath, Scan, Purge, Render, Apply
// and Check reaches it without another case in each switch.
const (
	PyPI   = "pypi"
	Cargo  = "cargo"
	Gem    = "gem"
	Gradle = "gradle"
)

// fileFormat is one tool's credential file: where it is, what it holds, and
// how to remove one host from it or merge a set into it while keeping every
// line authama does not own.
type fileFormat interface {
	kind() string // KindToken or KindBasic
	path(h Host) string
	scan(text string) []Installed
	purge(host, text string) string
	merge(entries []Credential, existing string) string
	check(c Credential) error
}

var formats = map[string]fileFormat{PyPI: pypirc{}, Cargo: cargoCreds{}, Gem: gemCreds{}, Gradle: gradleProps{}}

// TargetKind is a tool's credential kind: KindToken for one secret,
// KindBasic for a user name and a password, "" for a tool with several.
func TargetKind(target string) string {
	if f, ok := formats[target]; ok {
		return f.kind()
	}
	switch target {
	case NPM:
		return KindToken
	case Docker, Git, Netrc:
		return KindBasic
	}
	return ""
}

func noSpace(what string, vals ...string) error {
	for _, v := range vals {
		if hasSpaceOrControl(v) {
			return errors.New(what + " cannot contain whitespace")
		}
	}
	return nil
}

// lines splits text into lines without their endings.
func lines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.ReplaceAll(strings.TrimRight(text, "\n"), "\r\n", "\n"), "\n")
}

// --- .pypirc: twine and other PyPI uploaders -----------------------------------

// pypirc is ~/.pypirc. A host is the index's upload host: upload.pypi.org is
// the [pypi] section, test.pypi.org [testpypi], and any other host a section
// of its own name with a repository URL. A PyPI API token is the password,
// with the user name __token__. Installing (pip, uv) reads ~/.netrc instead.
type pypirc struct{}

func (pypirc) kind() string { return KindBasic }

func (pypirc) path(h Host) string { return filepath.Join(h.Home, ".pypirc") }

func (pypirc) check(c Credential) error {
	if strings.ContainsAny(c.Host, "[]/") {
		return errors.New("a PyPI host is a host name, without brackets or a path")
	}
	return noSpace("a PyPI user name and password", c.Username, c.Password)
}

func pypiSection(host string) (section, repository string) {
	switch host {
	case "upload.pypi.org", "pypi.org", "pypi":
		return "pypi", ""
	case "test.pypi.org", "testpypi":
		return "testpypi", "https://test.pypi.org/legacy/"
	}
	return host, "https://" + host + "/"
}

func pypiHost(section, repository string) string {
	switch section {
	case "pypi":
		return "upload.pypi.org"
	case "testpypi":
		return "test.pypi.org"
	}
	if repository != "" {
		r := strings.TrimPrefix(strings.TrimPrefix(repository, "https://"), "http://")
		if i := strings.IndexAny(r, "/:"); i >= 0 {
			r = r[:i]
		}
		if r != "" {
			return r
		}
	}
	return section
}

// iniSections reads an INI file into its sections' keys, in order.
func iniSections(text string) (order []string, vals map[string]map[string]string) {
	vals = map[string]map[string]string{}
	cur := ""
	for _, ln := range lines(text) {
		st := strings.TrimSpace(ln)
		if strings.HasPrefix(st, "[") && strings.HasSuffix(st, "]") {
			cur = strings.TrimSpace(st[1 : len(st)-1])
			if _, seen := vals[cur]; !seen {
				vals[cur] = map[string]string{}
				order = append(order, cur)
			}
			continue
		}
		if cur == "" || st == "" || strings.HasPrefix(st, "#") || strings.HasPrefix(st, ";") {
			continue
		}
		if k, v, ok := strings.Cut(st, "="); ok {
			vals[cur][strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return order, vals
}

func (pypirc) scan(text string) []Installed {
	order, vals := iniSections(text)
	var out []Installed
	for _, sec := range order {
		if sec == "distutils" {
			continue
		}
		v := vals[sec]
		if v["password"] == "" && v["username"] == "" {
			continue
		}
		out = append(out, mk(PyPI, pypiHost(sec, v["repository"]), KindBasic, v["username"], v["password"] != "", v["password"]))
	}
	return out
}

// iniDropSection removes a section and its lines.
func iniDropSection(text, section string) []string {
	var keep []string
	in := false
	for _, ln := range lines(text) {
		st := strings.TrimSpace(ln)
		if strings.HasPrefix(st, "[") && strings.HasSuffix(st, "]") {
			in = strings.TrimSpace(st[1:len(st)-1]) == section
		}
		if !in {
			keep = append(keep, ln)
		}
	}
	return keep
}

// distutilsServers rewrites index-servers to list exactly servers, keeping
// everything else in [distutils].
func distutilsServers(ls []string, servers []string) []string {
	var out []string
	in, written := false, false
	skipping := false
	block := func() []string {
		b := []string{"index-servers ="}
		for _, s := range servers {
			b = append(b, "    "+s)
		}
		return b
	}
	for _, ln := range ls {
		st := strings.TrimSpace(ln)
		isHeader := strings.HasPrefix(st, "[") && strings.HasSuffix(st, "]")
		if isHeader {
			if in && !written {
				out = append(out, block()...)
				written = true
			}
			in = strings.TrimSpace(st[1:len(st)-1]) == "distutils"
			skipping = false
			out = append(out, ln)
			continue
		}
		if in && strings.HasPrefix(strings.ToLower(st), "index-servers") {
			out, written, skipping = append(out, block()...), true, true
			continue
		}
		if skipping && ln != "" && (ln[0] == ' ' || ln[0] == '\t') {
			continue // a continuation line of the old index-servers
		}
		skipping = false
		out = append(out, ln)
	}
	if in && !written {
		out = append(out, block()...)
		written = true
	}
	if !written {
		out = append([]string{"[distutils]"}, append(block(), append([]string{""}, out...)...)...)
	}
	return out
}

func (p pypirc) purge(host, text string) string {
	sec, _ := pypiSection(host)
	ls := iniDropSection(text, sec)
	order, _ := iniSections(strings.Join(ls, "\n"))
	var servers []string
	for _, s := range order {
		if s != "distutils" {
			servers = append(servers, s)
		}
	}
	return strings.Join(distutilsServers(ls, servers), "\n")
}

func (p pypirc) merge(entries []Credential, existing string) string {
	text := existing
	for _, e := range entries {
		sec, _ := pypiSection(e.Host)
		text = strings.Join(iniDropSection(text, sec), "\n")
	}
	ls := lines(text)
	for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
		ls = ls[:len(ls)-1]
	}
	for _, e := range entries {
		sec, repo := pypiSection(e.Host)
		if len(ls) > 0 {
			ls = append(ls, "")
		}
		ls = append(ls, "["+sec+"]")
		if repo != "" {
			ls = append(ls, "repository = "+repo)
		}
		ls = append(ls, "username = "+e.Username, "password = "+e.Password)
	}
	order, _ := iniSections(strings.Join(ls, "\n"))
	var servers []string
	for _, s := range order {
		if s != "distutils" {
			servers = append(servers, s)
		}
	}
	return strings.Join(distutilsServers(ls, servers), "\n")
}

// --- Cargo: ~/.cargo/credentials.toml ----------------------------------------

// cargoCreds is $CARGO_HOME/credentials.toml. A host is a registry: crates.io
// is [registry], any other name [registries.<name>], as in .cargo/config.toml.
type cargoCreds struct{}

func (cargoCreds) kind() string { return KindToken }

func (cargoCreds) path(h Host) string {
	home := filepath.Join(h.Home, ".cargo")
	if v := h.env("CARGO_HOME"); v != "" {
		home = h.expandUser(v)
	}
	return filepath.Join(home, "credentials.toml")
}

func (cargoCreds) check(c Credential) error {
	if c.Host != "crates.io" && !cargoName(c.Host) {
		return errors.New("a Cargo registry is crates.io or a registry name: letters, digits, - and _")
	}
	return noSpace("a Cargo token", c.Token)
}

func cargoName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !isLetter(r) && !isDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func cargoTable(host string) string {
	if host == "crates.io" {
		return "registry"
	}
	return "registries." + host
}

func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func tomlUnquote(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, " #"); i >= 0 && !strings.HasPrefix(v, `"`) {
		v = v[:i]
	}
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		q := v[0]
		v = v[1 : len(v)-1]
		if q == '"' {
			v = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v)
		}
	}
	return v
}

func (c cargoCreds) scan(text string) []Installed {
	order, vals := iniSections(text)
	var out []Installed
	for _, t := range order {
		tok := tomlUnquote(vals[t]["token"])
		if tok == "" {
			continue
		}
		host := ""
		switch {
		case t == "registry":
			host = "crates.io"
		case strings.HasPrefix(t, "registries."):
			host = tomlUnquote(strings.TrimPrefix(t, "registries."))
		default:
			continue
		}
		out = append(out, mk(Cargo, host, KindToken, "", true, tok))
	}
	return out
}

func (c cargoCreds) purge(host, text string) string {
	return strings.Join(iniDropSection(text, cargoTable(host)), "\n")
}

func (c cargoCreds) merge(entries []Credential, existing string) string {
	text := existing
	for _, e := range entries {
		text = strings.Join(iniDropSection(text, cargoTable(e.Host)), "\n")
	}
	ls := lines(text)
	for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
		ls = ls[:len(ls)-1]
	}
	for _, e := range entries {
		if len(ls) > 0 {
			ls = append(ls, "")
		}
		ls = append(ls, "["+cargoTable(e.Host)+"]", "token = "+tomlString(e.Token))
	}
	return strings.Join(ls, "\n")
}

// --- RubyGems: ~/.gem/credentials --------------------------------------------

// gemCreds is ~/.gem/credentials, the flat YAML map `gem push` reads. A host
// is rubygems.org (the :rubygems_api_key key) or another gem server, keyed by
// its https:// URL. gem refuses the file unless it is owner-only, which every
// credential file authama writes is.
type gemCreds struct{}

func (gemCreds) kind() string { return KindToken }

func (gemCreds) path(h Host) string { return filepath.Join(h.Home, ".gem", "credentials") }

func (gemCreds) check(c Credential) error {
	if strings.ContainsAny(c.Host, ": ") && !strings.HasPrefix(c.Host, "https://") {
		return errors.New("a gem host is a host name, or an https:// URL")
	}
	if hasControl(c.Token) {
		return errors.New("a gem key cannot contain a control character")
	}
	return nil
}

func gemKey(host string) string {
	switch host {
	case "rubygems.org", "":
		return ":rubygems_api_key"
	}
	if strings.HasPrefix(host, "https://") {
		return strings.TrimRight(host, "/")
	}
	return "https://" + strings.TrimRight(host, "/")
}

func gemHost(key string) string {
	if key == ":rubygems_api_key" {
		return "rubygems.org"
	}
	return strings.TrimPrefix(key, "https://")
}

// gemEntries reads the map's top-level "key: value" lines.
func gemEntries(text string) (keys []string, vals map[string]string) {
	vals = map[string]string{}
	for _, ln := range lines(text) {
		if ln == "" || ln == "---" || strings.HasPrefix(ln, "#") || ln[0] == ' ' {
			continue
		}
		i := strings.LastIndex(ln, ": ")
		if i < 0 {
			continue
		}
		k, v := strings.Trim(ln[:i], `"'`), strings.Trim(strings.TrimSpace(ln[i+2:]), `"'`)
		if _, seen := vals[k]; !seen {
			keys = append(keys, k)
		}
		vals[k] = v
	}
	return keys, vals
}

func (g gemCreds) scan(text string) []Installed {
	keys, vals := gemEntries(text)
	var out []Installed
	for _, k := range keys {
		if k == ":rubygems_api_key" || strings.HasPrefix(k, "https://") {
			out = append(out, mk(Gem, gemHost(k), KindToken, "", vals[k] != "", vals[k]))
		}
	}
	return out
}

func (g gemCreds) dropKey(text, key string) []string {
	var keep []string
	for _, ln := range lines(text) {
		if i := strings.LastIndex(ln, ": "); i >= 0 && ln[0] != ' ' && strings.Trim(ln[:i], `"'`) == key {
			continue
		}
		keep = append(keep, ln)
	}
	return keep
}

func (g gemCreds) purge(host, text string) string {
	return strings.Join(g.dropKey(text, gemKey(host)), "\n")
}

func (g gemCreds) merge(entries []Credential, existing string) string {
	text := existing
	for _, e := range entries {
		text = strings.Join(g.dropKey(text, gemKey(e.Host)), "\n")
	}
	ls := lines(text)
	if len(ls) == 0 || strings.TrimSpace(ls[0]) != "---" {
		ls = append([]string{"---"}, ls...)
	}
	for _, e := range entries {
		k := gemKey(e.Host)
		if strings.HasPrefix(k, "https://") {
			k = `"` + k + `"`
		}
		ls = append(ls, k+": "+e.Token)
	}
	return strings.Join(ls, "\n")
}

// --- Gradle: gradle.properties -----------------------------------------------

// gradleProps is $GRADLE_USER_HOME/gradle.properties. A host is the name a
// build gives a repository whose credentials it reads with
// credentials(PasswordCredentials): repository "acme" reads acmeUsername and
// acmePassword.
type gradleProps struct{}

func (gradleProps) kind() string { return KindBasic }

func (gradleProps) path(h Host) string {
	home := filepath.Join(h.Home, ".gradle")
	if v := h.env("GRADLE_USER_HOME"); v != "" {
		home = h.expandUser(v)
	}
	return filepath.Join(home, "gradle.properties")
}

func (gradleProps) check(c Credential) error {
	for i, r := range c.Host {
		if !isLetter(r) && r != '_' && (i == 0 || !isDigit(r)) {
			return errors.New("a Gradle repository name is an identifier: letters, digits and _, not starting with a digit")
		}
	}
	return nil
}

func propEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "=", `\=`, ":", `\:`, "#", `\#`, "!", `\!`).Replace(s)
}

func propUnescape(s string) string {
	return strings.NewReplacer(`\\`, `\`, `\n`, "\n", `\=`, "=", `\:`, ":", `\#`, "#", `\!`, "!").Replace(s)
}

func propLines(text string) (keys []string, vals map[string]string) {
	vals = map[string]string{}
	for _, ln := range lines(text) {
		st := strings.TrimSpace(ln)
		if st == "" || st[0] == '#' || st[0] == '!' {
			continue
		}
		k, v, ok := strings.Cut(st, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		keys = append(keys, k)
		vals[k] = propUnescape(strings.TrimSpace(v))
	}
	return keys, vals
}

func (g gradleProps) scan(text string) []Installed {
	keys, vals := propLines(text)
	var out []Installed
	seen := map[string]bool{}
	for _, k := range keys {
		repo, ok := strings.CutSuffix(k, "Password")
		if !ok || repo == "" || seen[repo] {
			continue
		}
		seen[repo] = true
		out = append(out, mk(Gradle, repo, KindBasic, vals[repo+"Username"], vals[k] != "", vals[k]))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

func (g gradleProps) dropRepo(text, repo string) []string {
	var keep []string
	for _, ln := range lines(text) {
		k, _, _ := strings.Cut(strings.TrimSpace(ln), "=")
		k = strings.TrimSpace(k)
		if k == repo+"Username" || k == repo+"Password" {
			continue
		}
		keep = append(keep, ln)
	}
	return keep
}

func (g gradleProps) purge(host, text string) string {
	return strings.Join(g.dropRepo(text, host), "\n")
}

func (g gradleProps) merge(entries []Credential, existing string) string {
	text := existing
	for _, e := range entries {
		text = strings.Join(g.dropRepo(text, e.Host), "\n")
	}
	ls := lines(text)
	for _, e := range entries {
		ls = append(ls, fmt.Sprintf("%sUsername=%s", e.Host, propEscape(e.Username)),
			fmt.Sprintf("%sPassword=%s", e.Host, propEscape(e.Password)))
	}
	return strings.Join(ls, "\n")
}
