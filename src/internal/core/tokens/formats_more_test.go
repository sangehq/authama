package tokens

import (
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/testcred"
)

func cred(target, host, user, secret string) Credential {
	c := Credential{Target: target, Host: host, Kind: formats[target].kind()}
	if c.Kind == KindToken {
		c.Token = secret
	} else {
		c.Username, c.Password = user, secret
	}
	return c
}

func one(t *testing.T, target, text, host string) Installed {
	t.Helper()
	var got []Installed
	for _, in := range Scan(target, text) {
		if in.Host == host {
			got = append(got, in)
		}
	}
	if len(got) != 1 {
		t.Fatalf("%s: %d entries for %s in:\n%s", target, len(got), host, text)
	}
	return got[0]
}

// settings.xml is edited by splicing one <server> element: every other line,
// comment and element stays byte for byte, a second merge replaces rather
// than duplicates, a brace is escaped so Maven does not read it as an
// encrypted value, and purging gives back the file exactly as it was.
func TestMavenSettingsKeepEverythingButItsOwnServer(t *testing.T) {
	orig := `<?xml version="1.0" encoding="UTF-8"?>
<!-- the team's settings: keep this comment -->
<settings xmlns="http://maven.apache.org/SETTINGS/1.0.0">
  <mirrors>
    <mirror><id>corp</id><url>https://repo.example.com/maven</url><mirrorOf>*</mirrorOf></mirror>
  </mirrors>
  <servers>
    <server>
      <id>corp</id>
      <username>ci</username>
      <password>{COQLCE6DU6GtcS5P=}</password>
    </server>
  </servers>
</settings>
`
	pass := "a{b}" + testcred.Secret("maven")
	got := formats[Maven].merge([]Credential{cred(Maven, "acme-releases", "deploy", pass)}, orig)
	for _, keep := range []string{"<!-- the team's settings: keep this comment -->", "<mirrorOf>*</mirrorOf>", "{COQLCE6DU6GtcS5P=}"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("lost %q:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, `<password>a\{b\}`) {
		t.Fatalf("a brace was not escaped:\n%s", got)
	}
	in := one(t, Maven, got, "acme-releases")
	if in.Username != "deploy" || in.Secret != pass {
		t.Fatalf("scan read %q / %q", in.Username, in.Secret)
	}
	again := formats[Maven].merge([]Credential{cred(Maven, "acme-releases", "deploy", "second")}, got)
	if one(t, Maven, again, "acme-releases").Secret != "second" {
		t.Fatal("a second merge did not replace the server")
	}
	if back := formats[Maven].purge("acme-releases", again); back != orig {
		t.Fatalf("purge did not give the file back:\n%s", back)
	}
	if got := one(t, Maven, orig, "corp"); got.Secret != "{COQLCE6DU6GtcS5P=}" {
		t.Fatalf("an encrypted password read as %q", got.Secret)
	}

	fresh := formats[Maven].merge([]Credential{cred(Maven, "acme", "u", "p")}, "")
	if !strings.HasPrefix(fresh, "<?xml") || one(t, Maven, fresh, "acme").Secret != "p" {
		t.Fatalf("a new settings.xml:\n%s", fresh)
	}
	noServers := "<settings>\n  <localRepository>/tmp/m2</localRepository>\n</settings>\n"
	added := formats[Maven].merge([]Credential{cred(Maven, "acme", "u", "p")}, noServers)
	if !strings.Contains(added, "<localRepository>/tmp/m2</localRepository>") || one(t, Maven, added, "acme").Username != "u" {
		t.Fatalf("servers were not added to a file without them:\n%s", added)
	}
	if bad := "not xml at all"; formats[Maven].merge([]Credential{cred(Maven, "acme", "u", "p")}, bad) != bad {
		t.Fatal("a file that is not a settings document was changed")
	}
	if Check(cred(Maven, "has space", "u", "p")) == nil || Check(cred(Maven, "acme_repo-1.x", "u", "p")) != nil {
		t.Fatal("the server id check is wrong")
	}
}

func TestYarnrcKeepsOtherKeysAndMatchesEitherSpelling(t *testing.T) {
	orig := `# managed by hand
nodeLinker: node-modules
npmScopes:
  acme:
    npmRegistryServer: "https://npm.pkg.github.com"
npmRegistries:
  "https://registry.example.com":
    npmAlwaysAuth: false
    npmAuthToken: old
`
	tok := testcred.Secret("yarn")
	got := formats[Yarn].merge([]Credential{
		cred(Yarn, "npm.pkg.github.com", "", tok),
		cred(Yarn, "registry.example.com", "", "new"),
	}, orig)
	for _, keep := range []string{"# managed by hand", "nodeLinker: node-modules", "npmRegistryServer:"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("lost %q:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, `"//npm.pkg.github.com":`) || strings.Count(got, "registry.example.com") != 1 {
		t.Fatalf("keys are wrong (an existing https:// key must be updated, not duplicated):\n%s", got)
	}
	if one(t, Yarn, got, "npm.pkg.github.com").Secret != tok || one(t, Yarn, got, "registry.example.com").Secret != "new" {
		t.Fatalf("scan:\n%s", got)
	}
	purged := formats[Yarn].purge("npm.pkg.github.com", got)
	if strings.Contains(purged, `"//npm.pkg.github.com"`) || !strings.Contains(purged, "registry.example.com") ||
		!strings.Contains(purged, `npmRegistryServer: "https://npm.pkg.github.com"`) {
		t.Fatalf("purge:\n%s", purged)
	}
	if bad := "- a\n- list\n"; formats[Yarn].merge([]Credential{cred(Yarn, "x.example", "", "t")}, bad) != bad {
		t.Fatal("a file that is not a mapping was changed")
	}
}

func TestHelmRepositoriesUpdateTheRepoHelmAddedOrAddOne(t *testing.T) {
	orig := `apiVersion: ""
generated: "2026-10-01T10:00:00Z"
repositories:
- caFile: ""
  name: acme
  url: https://charts.example.com/stable/
  insecure_skip_tls_verify: false
`
	pass := testcred.Secret("helm")
	got := formats[Helm].merge([]Credential{
		cred(Helm, "https://charts.example.com/stable", "deploy", pass),
		cred(Helm, "https://other.example.org/charts", "u", "p"),
	}, orig)
	if !strings.Contains(got, `generated: "2026-10-01T10:00:00Z"`) || !strings.Contains(got, "name: acme") ||
		strings.Count(got, "charts.example.com") != 1 {
		t.Fatalf("the existing repository was not updated in place:\n%s", got)
	}
	if !strings.Contains(got, "other.example.org-charts") {
		t.Fatalf("a new repository has no derived name:\n%s", got)
	}
	in := one(t, Helm, got, "https://charts.example.com/stable/")
	if in.Username != "deploy" || in.Secret != pass {
		t.Fatalf("scan: %+v", in)
	}
	purged := formats[Helm].purge("https://charts.example.com/stable", got)
	if strings.Contains(purged, pass) || !strings.Contains(purged, "name: acme") {
		t.Fatalf("purge must drop the credentials and keep the repository:\n%s", purged)
	}
	if Check(cred(Helm, "charts.example.com", "u", "p")) == nil {
		t.Fatal("a repository without a scheme was accepted")
	}
}

func TestHelmOCIRegistryIsDockerShapedAndKeepsTheCredsStore(t *testing.T) {
	orig := `{
  "credsStore": "osxkeychain",
  "auths": {
    "registry.example.com": {}
  }
}`
	pass := testcred.Secret("oci")
	got := formats[HelmOCI].merge([]Credential{cred(HelmOCI, "ghcr.io", "me", pass)}, orig)
	if !strings.Contains(got, `"credsStore": "osxkeychain"`) || !strings.Contains(got, "registry.example.com") {
		t.Fatalf("lost a key:\n%s", got)
	}
	in := one(t, HelmOCI, got, "ghcr.io")
	if in.Target != HelmOCI || in.Username != "me" || in.Secret != pass {
		t.Fatalf("scan: %+v", in)
	}
	if strings.Contains(formats[HelmOCI].purge("ghcr.io", got), "ghcr.io") {
		t.Fatal("purge kept the registry")
	}
	if Check(cred(HelmOCI, "oci://ghcr.io/acme", "u", "p")) == nil {
		t.Fatal("an oci:// reference was accepted as a host")
	}
}

// gh reads a token kept out of the keyring from users.<login>.oauth_token,
// and uses the host's own oauth_token for the active account; both are
// written, other accounts and settings on the host are kept.
func TestGHHostsWritesTheAccountAndKeepsTheOthers(t *testing.T) {
	orig := `github.com:
    git_protocol: ssh
    users:
        other:
            oauth_token: keep-me
    user: other
    oauth_token: keep-me
ghe.example.com:
    user: ent
`
	tok := testcred.Secret("gh")
	got := formats[GH].merge([]Credential{cred(GH, "github.com", "alice", tok)}, orig)
	for _, keep := range []string{"git_protocol: ssh", "other:", "keep-me", "ghe.example.com:"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("lost %q:\n%s", keep, got)
		}
	}
	in := one(t, GH, got, "github.com")
	if in.Username != "alice" || in.Secret != tok {
		t.Fatalf("scan: %+v\n%s", in, got)
	}
	if strings.Count(got, tok) != 2 {
		t.Fatalf("the token must be the account's and the host's:\n%s", got)
	}
	fresh := formats[GH].merge([]Credential{cred(GH, "github.com", "alice", tok)}, "")
	if !strings.Contains(fresh, `git_protocol: "https"`) {
		t.Fatalf("a new host has no git_protocol:\n%s", fresh)
	}
	purged := formats[GH].purge("github.com", got)
	if strings.Contains(purged, tok) || !strings.Contains(purged, "keep-me") {
		t.Fatalf("purge must drop alice's token only:\n%s", purged)
	}
	if Check(cred(GH, "github.com", "", tok)) == nil {
		t.Fatal("a token without the account's login was accepted")
	}
}

func TestTheNewTargetsFindTheirFiles(t *testing.T) {
	env := map[string]string{}
	h := Host{Home: "/h", Getenv: func(k string) string { return env[k] }}
	for target, want := range map[string]string{
		Maven: "/h/.m2/settings.xml", Yarn: "/h/.yarnrc.yml",
		Helm: "/h/.config/helm/repositories.yaml", HelmOCI: "/h/.config/helm/registry/config.json",
		GH: "/h/.config/gh/hosts.yml",
	} {
		if got := strings.ReplaceAll(h.TargetPath(target, ""), `\`, "/"); got != want {
			t.Errorf("%s: %s, want %s", target, got, want)
		}
	}
	mac := Host{Home: "/h", MacOS: true, Getenv: h.Getenv}
	if got := strings.ReplaceAll(mac.TargetPath(Helm, ""), `\`, "/"); got != "/h/Library/Preferences/helm/repositories.yaml" {
		t.Errorf("helm on macOS: %s", got)
	}
	env["GH_CONFIG_DIR"] = "/x/gh"
	env["HELM_REGISTRY_CONFIG"] = "/x/reg.json"
	if got := strings.ReplaceAll(h.TargetPath(GH, ""), `\`, "/"); got != "/x/gh/hosts.yml" {
		t.Errorf("GH_CONFIG_DIR: %s", got)
	}
	if got := strings.ReplaceAll(h.TargetPath(HelmOCI, ""), `\`, "/"); got != "/x/reg.json" {
		t.Errorf("HELM_REGISTRY_CONFIG: %s", got)
	}
	for _, v := range []string{"HELM_CONFIG_HOME", "HELM_REPOSITORY_CONFIG", "HELM_REGISTRY_CONFIG", "GH_CONFIG_DIR"} {
		found := false
		for _, o := range PathOverrideVars {
			found = found || o == v
		}
		if !found {
			t.Errorf("%s is not hidden in dev mode", v)
		}
	}
}
