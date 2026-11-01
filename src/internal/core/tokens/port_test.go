package tokens

// Ports of auth-token-manager's pure tests (test_core.py, test_catalog.py,
// test_templates.py, test_windows.py and the text half of test_aliases.py).
// The file-touching ones live in internal/services/tokensvc.

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func tok(target, host, token string) Credential {
	return Credential{Target: target, Host: host, Kind: KindToken, Mechanism: "github-oauth", Token: token}
}

func bas(target, host, user, pass string) Credential {
	return Credential{Target: target, Host: host, Kind: KindBasic, Mechanism: "http-basic", Username: user, Password: pass}
}

func composerOf(t *testing.T, s *Set) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s.ComposerJSON()), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTargetsComeOutInInsertionOrder(t *testing.T) {
	s := NewSet()
	s.Add(tok(Composer, "github.com", "tok"))
	s.Add(Credential{Target: NPM, Host: "registry.npmjs.org", Kind: KindToken, Token: "n"})
	if !slices.Equal(s.Targets(), []string{"composer", "npm"}) {
		t.Fatal(s.Targets())
	}
}

func TestEmptySecretsAreSkippedAndWhitespaceTrimmed(t *testing.T) {
	s := NewSet()
	if s.Add(tok(Composer, "x.com", "")) || s.Add(tok(Composer, "y.com", "  \n")) || s.Len("") != 0 {
		t.Fatal("kept an empty secret")
	}
	s.Add(tok(Composer, "github.com", "  tok\n"))
	if composerOf(t, s)["github-oauth"].(map[string]any)["github.com"] != "tok" {
		t.Fatal(s.ComposerJSON())
	}
}

func TestBasicNeedsBothParts(t *testing.T) {
	if NewSet().Add(bas(Composer, "h", "user", "")) || NewSet().Add(bas(Composer, "h", "", "pw")) || !NewSet().Add(bas(Composer, "h", "user", "pw")) {
		t.Fatal("basic completeness")
	}
}

func TestTheLastWriteWins(t *testing.T) {
	s := NewSet()
	s.Add(tok(Composer, "github.com", "a"))
	s.Add(tok(Composer, "github.com", "b"))
	if s.Len("") != 1 || composerOf(t, s)["github-oauth"].(map[string]any)["github.com"] != "b" {
		t.Fatal(s.ComposerJSON())
	}
}

func TestRemove(t *testing.T) {
	s := NewSet()
	s.Add(Credential{Target: NPM, Host: "r", Kind: KindToken, Token: "t"})
	if !s.Remove(NPM, "r") || s.Remove(NPM, "r") || s.Len("") != 0 {
		t.Fatal("remove")
	}
}

func TestComposerShapes(t *testing.T) {
	s := NewSet()
	s.Add(tok(Composer, "github.com", "g"))
	s.Add(bas(Composer, "repo.magento.com", "pub", "priv"))
	s.Add(Credential{Target: Composer, Host: "bitbucket.org", Kind: KindPair, Mechanism: "bitbucket-oauth", Key: "k", Secret: "s"})
	s.Add(Credential{Target: Composer, Host: "h.org", Kind: KindHeaders, Mechanism: "custom-headers", Headers: []string{"API-TOKEN: x"}})
	want := `{"bitbucket-oauth":{"bitbucket.org":{"consumer-key":"k","consumer-secret":"s"}},` +
		`"custom-headers":{"h.org":["API-TOKEN: x"]},"github-oauth":{"github.com":"g"},` +
		`"http-basic":{"repo.magento.com":{"password":"priv","username":"pub"}}}`
	got, _ := json.Marshal(composerOf(t, s))
	if string(got) != want {
		t.Fatal(string(got))
	}
}

func TestNPMRCWithAScope(t *testing.T) {
	s := NewSet()
	s.Add(Credential{Target: NPM, Host: "registry.npmjs.org", Kind: KindToken, Token: "tok", Scope: "@org"})
	out := s.NPMRC()
	if !strings.Contains(out, "@org:registry=https://registry.npmjs.org/") || !strings.Contains(out, "//registry.npmjs.org/:_authToken=tok") {
		t.Fatal(out)
	}
}

func TestDockerAuthIsBase64(t *testing.T) {
	s := NewSet()
	s.Add(bas(Docker, "ghcr.io", "user", "pat"))
	if !strings.Contains(s.DockerJSON(), base64.StdEncoding.EncodeToString([]byte("user:pat"))) {
		t.Fatal(s.DockerJSON())
	}
}

func TestGitCredentialsEncodeSecretsAndKeepTheScheme(t *testing.T) {
	s := NewSet()
	s.Add(bas(Git, "github.com", "u@s", "changeme/x"))
	if got, want := s.GitCredentials(), "https://"+strings.Join([]string{"u%40s", "changeme%2Fx"}, ":")+"@github.com"; got != want {
		t.Fatal(got)
	}
	for host, want := range map[string]string{"https://github.com": "https://u:p@github.com", "http://git.local": "http://u:p@git.local"} {
		s := NewSet()
		s.Add(bas(Git, host, "u", "p"))
		if s.GitCredentials() != want {
			t.Fatal(s.GitCredentials())
		}
	}
}

func TestNetrcLine(t *testing.T) {
	s := NewSet()
	s.Add(bas(Netrc, "api.heroku.com", "u", "k"))
	if s.NetrcText() != "machine api.heroku.com login u password k" {
		t.Fatal(s.NetrcText())
	}
}

func TestRedactedJSONHidesSecretsAndFiltersTargets(t *testing.T) {
	s := NewSet()
	s.Add(tok(Composer, "github.com", "supersecret"))
	s.Add(Credential{Target: NPM, Host: "registry.npmjs.org", Kind: KindToken, Token: "ntok"})
	all := s.RedactedJSON(nil)
	if strings.Contains(all, "supersecret") || !strings.Contains(all, Mask) {
		t.Fatal(all)
	}
	var doc map[string]any
	_ = json.Unmarshal([]byte(s.RedactedJSON(map[string]bool{"npm": true})), &doc)
	if _, ok := doc["composer"]; ok || doc["npm"] == nil {
		t.Fatal(doc)
	}
}

func resolveDoc(t *testing.T, doc string, env map[string]string) *Set {
	t.Helper()
	c, err := ParseConfig(doc)
	if err != nil {
		t.Fatal(err)
	}
	return c.Resolve(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
}

func TestConfigResolvesAndWarns(t *testing.T) {
	s := resolveDoc(t, `{"composer":{"github.com":{"service":"github","env":"GH_TOKEN"},"x.org":{"service":"gitlab","env":"UNSET_X"}},"npm":{"r":{"env":"UNSET_X"}}}`,
		map[string]string{"GH_TOKEN": "ghp_x"})
	if composerOf(t, s)["github-oauth"].(map[string]any)["github.com"] != "ghp_x" || !strings.Contains(strings.Join(s.Warnings, "\n"), "x.org") {
		t.Fatal(s.Warnings)
	}
}

func TestABareConfigIsComposer(t *testing.T) {
	s := resolveDoc(t, `{"github.com":{"service":"github","env":"GH_TOKEN"}}`, map[string]string{"GH_TOKEN": "x"})
	if !slices.Equal(s.Targets(), []string{"composer"}) {
		t.Fatal(s.Targets())
	}
}

func TestAnUnknownServiceWarns(t *testing.T) {
	s := resolveDoc(t, `{"composer":{"h":{"service":"nope","env":"X"}}}`, nil)
	if s.Len("") != 0 || !strings.Contains(strings.Join(s.Warnings, "\n"), "unknown service") {
		t.Fatal(s.Warnings)
	}
}

func TestExpandEnvPresentButEmpty(t *testing.T) {
	get := func(k string) (string, bool) {
		if k == "EMPTY" {
			return "", true
		}
		return "", false
	}
	if v, ok := ExpandEnv("a${EMPTY}b", get); !ok || v != "ab" {
		t.Fatal(v, ok)
	}
	if _, ok := ExpandEnv("a${GONE}b", get); ok {
		t.Fatal("absent var expanded")
	}
}

func TestValidateEntry(t *testing.T) {
	spec := func(target, body string) Spec {
		c, _ := ParseConfig(`{"` + target + `":{"h":` + body + `}}`)
		return c.Specs()[0]
	}
	if ValidateEntry(spec("composer", `{"service":"github","env":"X"}`)) != "" ||
		ValidateEntry(spec("composer", `{"service":"github"}`)) == "" ||
		ValidateEntry(spec("docker", `{"user_env":"U","pass_env":"P"}`)) != "" ||
		ValidateEntry(spec("docker", `{"user_env":"U"}`)) == "" {
		t.Fatal("validate_entry")
	}
}

var realMechanisms = []string{"github-oauth", "gitlab-oauth", "gitlab-token", "bitbucket-oauth",
	"http-basic", "bearer", "custom-headers", "forgejo-token"}

func TestTheCatalogIsConsistent(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.Targets, []string{"composer", "npm", "docker", "git", "netrc"}) {
		t.Fatal(c.Targets)
	}
	for svc, mech := range c.Services {
		if !slices.Contains(realMechanisms, mech) {
			t.Fatalf("%s -> %s", svc, mech)
		}
	}
	for m := range c.TokenMechanisms {
		if !slices.Contains(realMechanisms, m) {
			t.Fatal(m)
		}
	}
	for svc := range c.Enterprise {
		if _, ok := c.Services[svc]; !ok {
			t.Fatal(svc)
		}
	}
	var menu []string
	for _, m := range c.Menu {
		menu = append(menu, m.Key)
	}
	slices.Sort(menu)
	want := Targets()
	slices.Sort(want)
	if !slices.Equal(menu, want) || len(c.TargetFields) != len(want) {
		t.Fatal(menu)
	}
	for target, rows := range c.Providers {
		if !IsTarget(target) {
			t.Fatal(target)
		}
		for _, p := range rows {
			if p.Key == "" || p.Label == "" || !p.Hostable && p.Cloud == "" {
				t.Fatalf("%+v", p)
			}
		}
	}
	if !strings.Contains(c.DefaultConfig, `"github.com"`) || !strings.Contains(c.DefaultConfig, `"composer"`) {
		t.Fatal(c.DefaultConfig)
	}
}

func section(t *testing.T, text, key string) string {
	t.Helper()
	doc, err := decodeOrdered(text)
	if err != nil {
		t.Fatal(err)
	}
	return encodeCompact(doc.(*object).vals[key])
}

func TestTheAllTargetsTemplateIsTheUnionOfThePerToolOnes(t *testing.T) {
	all := AllTargetsTemplate()
	for _, target := range Targets() {
		c, err := TargetTemplate(target)
		if err != nil {
			t.Fatal(err)
		}
		if section(t, all, target) != section(t, c.JSON(), target) {
			t.Fatalf("%s drifted from its template", target)
		}
		// each resolves without error, with nothing set
		_ = c.Resolve(func(string) (string, bool) { return "", false })
	}
	if _, err := TargetTemplate("nope"); err == nil {
		t.Fatal("unknown template accepted")
	}
}

func TestTheComposerTemplateCoversEveryService(t *testing.T) {
	c, _ := TargetTemplate(Composer)
	used := map[string]bool{}
	for _, sp := range c.Specs() {
		used[sp.Service] = true
	}
	for _, svc := range cat().ServiceNames() {
		if !used[svc] {
			t.Fatalf("composer template is missing %s", svc)
		}
	}
}

func TestAStarterConfigHasHelpAndTheChosenTargets(t *testing.T) {
	c, err := StarterConfig([]string{"npm", "docker"})
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := decodeOrdered(c.JSON())
	if keys := doc.(*object).keys; !slices.Equal(keys, []string{"_help", "npm", "docker"}) {
		t.Fatal(keys)
	}
	if !strings.Contains(c.JSON(), "registry.npmjs.org") {
		t.Fatal(c.JSON())
	}
}

func host(env map[string]string, windows bool, files ...string) Host {
	return Host{
		Home: "/home/u", Windows: windows,
		Getenv: func(k string) string { return env[k] },
		Exists: func(p string) bool { return slices.Contains(files, p) },
		IsDir:  func(string) bool { return false },
	}
}

func TestDefaultPathsAndOverrides(t *testing.T) {
	h := host(nil, false)
	for _, target := range Targets() {
		if h.TargetPath(target, "") == "" {
			t.Fatal(target)
		}
	}
	tmp := "/tmp/x"
	h = host(map[string]string{"NPM_CONFIG_USERCONFIG": tmp + "/custom.npmrc", "DOCKER_CONFIG": tmp + "/dkr",
		"NETRC": tmp + "/mynetrc", "COMPOSER_HOME": tmp}, false)
	for target, want := range map[string]string{NPM: tmp + "/custom.npmrc", Docker: tmp + "/dkr/config.json", Netrc: tmp + "/mynetrc"} {
		if got := h.TargetPath(target, ""); got != filepath.FromSlash(want) {
			t.Fatal(target, got)
		}
	}
	if h.ComposerHome() != filepath.FromSlash(tmp) {
		t.Fatal(h.ComposerHome())
	}
	if got := h.TargetPath(NPM, "/proj"); got != filepath.FromSlash("/proj/.npmrc") {
		t.Fatal(got)
	}
}

func TestGitFallsBackToTheXDGFile(t *testing.T) {
	xdg := filepath.FromSlash("/x/git/credentials")
	h := host(map[string]string{"XDG_CONFIG_HOME": "/x"}, false)
	if got := h.TargetPath(Git, ""); got != filepath.Join("/home/u", ".git-credentials") {
		t.Fatal(got)
	}
	h = host(map[string]string{"XDG_CONFIG_HOME": "/x"}, false, xdg)
	if got := h.TargetPath(Git, ""); got != xdg {
		t.Fatal(got)
	}
}

func TestWindowsPaths(t *testing.T) {
	h := host(map[string]string{"APPDATA": "/appdata"}, true)
	if filepath.Base(h.TargetPath(Netrc, "")) != "_netrc" {
		t.Fatal(h.TargetPath(Netrc, ""))
	}
	if h.ComposerHome() != filepath.Join("/appdata", "Composer") {
		t.Fatal(h.ComposerHome())
	}
}

func TestEnvExportShellVariants(t *testing.T) {
	for _, c := range [][3]string{
		{"a'b", ShellPOSIX, `export V='a'\''b'`},
		{"x", ShellPowerShell, `$env:V = 'x'`},
		{"x", ShellCmd, `set "V=x"`},
		{"x", ShellFish, `set -gx V 'x'`},
		{`a\`, ShellFish, `set -gx V 'a\\'`},
		{"a'b", ShellFish, `set -gx V 'a\'b'`},
		{`{"a":"b&c"}`, ShellCmd, `set V={^"a^":^"b^&c^"}`},
		{`"a|b" > "c^d"`, ShellCmd, `set V=^"a^|b^" ^> ^"c^^d^"`},
	} {
		if got := EnvExport("V", c[0], c[1]); got != c[2] {
			t.Fatalf("%s %q: %s", c[1], c[0], got)
		}
	}
}

func TestDetectShellPowerShell(t *testing.T) {
	env := map[string]string{"PSModulePath": `C:\Modules`}
	if DetectShell(func(k string) string { return env[k] }, true) != ShellPowerShell {
		t.Fatal("not powershell")
	}
}

func TestAGitHostWithABadPortStillKeys(t *testing.T) {
	if got := GitKey("https://USER:pw@Git.Acme:8x43/Org/"); got != "git.acme:8x43/Org" {
		t.Fatal(got)
	}
}

// Security fix 2, inverted from test_malformed_composer_is_replaced_with_warning:
// a malformed auth.json is refused, never replaced.
func TestAMalformedComposerFileIsRefused(t *testing.T) {
	s := NewSet()
	s.Add(tok(Composer, "github.com", "x"))
	if _, err := s.Render(Composer, "{ not json", true); err == nil {
		t.Fatal("replaced a malformed auth.json")
	}
	if _, err := s.Render(Composer, "[1, 2]", true); err == nil {
		t.Fatal("replaced a non-object auth.json")
	}
}

// test_aliases.py, text transforms.

func TestAliasBlockTransforms(t *testing.T) {
	block := AliasBlock(DefaultAliases("authama"), "authama")
	if !strings.HasPrefix(block, AliasBegin) || !strings.HasSuffix(strings.TrimRight(block, "\n"), AliasEnd) ||
		!strings.Contains(block, "alias atm='authama token'") {
		t.Fatal(block)
	}
	once := RenderAliasInstall("export FOO=1\nalias ll='ls -l'\n", block)
	if !strings.Contains(once, "export FOO=1") || !strings.Contains(once, "alias ll='ls -l'") || strings.Count(once, AliasBegin) != 1 {
		t.Fatal(once)
	}
	if RenderAliasInstall(once, block) != once {
		t.Fatal("install is not idempotent")
	}
	second := RenderAliasInstall(RenderAliasInstall("", AliasBlock([]Alias{{"atm", "old-cmd"}}, "x")), AliasBlock([]Alias{{"atm", "new-cmd"}}, "x"))
	if strings.Contains(second, "old-cmd") || !strings.Contains(second, "alias atm='new-cmd'") {
		t.Fatal(second)
	}
	original := "line1\nline2\n"
	if got := RenderAliasRemove(RenderAliasInstall(original, block)); got != original {
		t.Fatalf("%q", got)
	}
	if StripAliasBlock("nothing here\n") != "nothing here" {
		t.Fatal("strip changed an unrelated file")
	}
	if !strings.Contains(AliasBlock([]Alias{{"atm", "/path with space/authama"}}, "x"), "alias atm='/path with space/authama'") {
		t.Fatal("not quoted")
	}
	lines := strings.Join(AliasLines(DefaultAliases("authama")), "\n")
	if strings.Contains(lines, AliasBegin) || !strings.Contains(lines, "alias atm='authama token'") {
		t.Fatal(lines)
	}
}

func TestTheDefaultProfileIsTheRcFile(t *testing.T) {
	no := func(string) bool { return false }
	for shell, want := range map[string]string{"/bin/zsh": ".zshrc", "/bin/bash": ".bashrc"} {
		if filepath.Base(DefaultProfile("/h", shell, false, no)) != want {
			t.Fatal(shell)
		}
	}
	if filepath.Base(DefaultProfile("/h", "/bin/zsh", true, no)) != ".zprofile" {
		t.Fatal("login")
	}
}
