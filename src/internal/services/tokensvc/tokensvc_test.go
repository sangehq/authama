package tokensvc

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/simtabi/ssh-manager/src/v3/internal/core/tokens"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/paths"
)

// sandbox is a token environment whose home, config and cwd are all under a
// temp dir, with no variable relocating a target.
func sandbox(t *testing.T, vars ...string) (Env, string) {
	t.Helper()
	dir := t.TempDir()
	env := map[string]string{"HOME": dir, "USERPROFILE": dir}
	for i := 0; i+1 < len(vars); i += 2 {
		env[vars[i]] = vars[i+1]
	}
	get := func(k string) string { return env[k] }
	look := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	p := paths.Paths{SSHDir: filepath.Join(dir, ".ssh"), ConfigDir: filepath.Join(dir, "cfg")}
	e := NewEnv(p, get, look, dir)
	e.Windows, e.MacOS = false, false
	return e, dir
}

func set(cs ...tokens.Credential) *tokens.Set {
	s := tokens.NewSet()
	for _, c := range cs {
		s.Add(c)
	}
	return s
}

func gh(host, tok string) tokens.Credential {
	return tokens.Credential{Target: tokens.Composer, Host: host, Kind: tokens.KindToken, Mechanism: "github-oauth", Token: tok}
}

func basic(target, host, user, pass string) tokens.Credential {
	return tokens.Credential{Target: target, Host: host, Kind: tokens.KindBasic, Username: user, Password: pass}
}

func npm(host, tok, scope string) tokens.Credential {
	return tokens.Credential{Target: tokens.NPM, Host: host, Kind: tokens.KindToken, Token: tok, Scope: scope}
}

func write(t *testing.T, e Env, s *tokens.Set, target string) string {
	t.Helper()
	p := e.TargetPath(target, "")
	if _, err := e.WriteTarget(s, target, p); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func installAll(t *testing.T, e Env) {
	c := gh("github.com", "ghp")
	hb := basic(tokens.Composer, "repo.x", "u", "pw")
	hb.Mechanism = "http-basic"
	write(t, e, set(c, hb), tokens.Composer)
	write(t, e, set(npm("registry.npmjs.org", "ntok", "")), tokens.NPM)
	write(t, e, set(basic(tokens.Docker, "ghcr.io", "octocat", "dpat")), tokens.Docker)
	write(t, e, set(basic(tokens.Git, "gitlab.com", "me", "glpat")), tokens.Git)
	write(t, e, set(basic(tokens.Netrc, "api.heroku.com", "login", "key")), tokens.Netrc)
}

func hostsOf(e Env, target string) []string {
	var out []string
	for _, it := range e.Inventory("", false) {
		if it.Target == target {
			out = append(out, it.Host)
		}
	}
	slices.Sort(out)
	return out
}

// test_writers.py

func TestAWrittenFileIsOwnerOnly(t *testing.T) {
	e, dir := sandbox(t)
	dest := filepath.Join(dir, "auth.json")
	if _, err := e.WriteTarget(set(gh("github.com", "tok")), tokens.Composer, dest); err != nil {
		t.Fatal(err)
	}
	if !ownerOnly(dest) {
		t.Fatal("not owner-only")
	}
	if fi, _ := os.Stat(dest); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", fi.Mode().Perm())
	}
}

func TestAWriteLeavesNoTempFileAndOverwrites(t *testing.T) {
	e, dir := sandbox(t)
	dest := filepath.Join(dir, "f")
	for _, s := range []string{"one", "two"} {
		if _, err := e.writeSecret(dest, s); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, dest); got != "two\n" {
		t.Fatalf("%q", got)
	}
	ents, _ := os.ReadDir(dir)
	for _, en := range ents {
		if en.Name() != "f" {
			t.Fatalf("leftover %s", en.Name())
		}
	}
}

func TestCheckupOfAnEmptyHomeIsClean(t *testing.T) {
	e, _ := sandbox(t)
	out, failed := e.Checkup(tokens.NewSet(), "")
	if failed || len(out) != 5 || out[0].Level != "absent" {
		t.Fatalf("%v %+v", failed, out)
	}
}

func TestCheckupFlagsAWorldReadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	e, dir := sandbox(t)
	bad := filepath.Join(dir, ".composer", "auth.json")
	_ = os.MkdirAll(filepath.Dir(bad), 0o700)
	_ = os.WriteFile(bad, []byte("{}"), 0o644)
	_ = os.Chmod(bad, 0o644)
	out, failed := e.Checkup(set(gh("github.com", "x")), "")
	if !failed || out[0].Level != "warn" || !strings.Contains(out[0].Text, "perms=0o644") {
		t.Fatalf("%+v", out)
	}
	if out[len(out)-1].Text != "a present auth.json OVERRIDES $COMPOSER_AUTH." {
		t.Fatalf("%+v", out)
	}
}

func TestCheckupFlagsAGitignoreMissingSecretFiles(t *testing.T) {
	e, dir := sandbox(t)
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("# x\nauth.json\n/.npmrc\n"), 0o644)
	out, failed := e.Checkup(tokens.NewSet(), "")
	last := out[len(out)-1]
	if !failed || !strings.HasSuffix(last.Text, "is missing: .git-credentials, .netrc") {
		t.Fatalf("%+v", last)
	}
}

func TestSourceLoadsAnEnvFileWithoutOverriding(t *testing.T) {
	e, dir := sandbox(t, "KEEP", "shell")
	f := filepath.Join(dir, ".env")
	_ = os.WriteFile(f, []byte("GH_TOKEN=test-secret-not-real\nKEEP=file\n"), 0o600)
	if err := e.LoadEnvFile(f); err != nil {
		t.Fatal(err)
	}
	if v, _ := e.Lookup("GH_TOKEN"); v != "test-secret-not-real" {
		t.Fatal(v)
	}
	if v, _ := e.Lookup("KEEP"); v != "shell" {
		t.Fatal(v)
	}
	if err := e.LoadEnvFile(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("missing --source accepted")
	}
}

func TestScaffoldNeverOverwrites(t *testing.T) {
	e, dir := sandbox(t)
	dest := filepath.Join(dir, "new.json")
	msg, err := e.Scaffold(dest, nil)
	if err != nil || !strings.Contains(msg, "all-targets.example.json") {
		t.Fatal(msg, err)
	}
	if _, err := e.Scaffold(dest, nil); err == nil {
		t.Fatal("overwrote")
	}
	two := filepath.Join(dir, "two.json")
	msg, err = e.Scaffold(two, []string{"npm", "git"})
	if err != nil || msg != "wrote npm+git config -> "+two {
		t.Fatal(msg, err)
	}
	c, err := LoadConfig(two)
	if err != nil {
		t.Fatal(err)
	}
	var targets []string
	for _, sp := range c.Specs() {
		if !slices.Contains(targets, sp.Target) {
			targets = append(targets, sp.Target)
		}
	}
	if !slices.Equal(targets, []string{"npm", "git"}) {
		t.Fatal(targets)
	}
}

// test_inventory.py

func TestInventoryListsEveryTarget(t *testing.T) {
	e, _ := sandbox(t)
	installAll(t, e)
	var got []string
	for _, it := range e.Inventory("", false) {
		got = append(got, it.Target+" "+it.Host)
		if it.File == "" || it.Description == "" {
			t.Fatalf("%+v", it)
		}
	}
	for _, want := range []string{"composer github.com", "composer repo.x", "npm registry.npmjs.org",
		"docker ghcr.io", "git gitlab.com", "netrc api.heroku.com"} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing %s in %v", want, got)
		}
	}
}

func TestInventoryMasksByDefaultAndRevealsOnRequest(t *testing.T) {
	e, _ := sandbox(t)
	write(t, e, set(gh("github.com", "ghp_SECRET")), tokens.Composer)
	m := e.Inventory("", false)[0]
	if !m.SecretPresent || m.Secret != "" {
		t.Fatalf("%+v", m)
	}
	if r := e.Inventory("", true)[0]; r.Secret != "ghp_SECRET" {
		t.Fatalf("%+v", r)
	}
}

func TestInventoryDescribesAndDecodesDocker(t *testing.T) {
	e, _ := sandbox(t)
	installAll(t, e)
	for _, it := range e.Inventory("", false) {
		if it.Target == tokens.Composer && it.Host == "github.com" &&
			(!strings.Contains(it.Description, "GitHub") || !strings.Contains(it.Description, "Composer")) {
			t.Fatal(it.Description)
		}
		if it.Target == tokens.Docker {
			if it.Username != "octocat" || !strings.HasSuffix(it.File, "config.json") || !strings.Contains(it.Description, "Docker") {
				t.Fatalf("%+v", it)
			}
		}
	}
}

func TestForgetRemovesOneHost(t *testing.T) {
	e, _ := sandbox(t)
	write(t, e, set(basic(tokens.Git, "github.com", "u", "t1"), basic(tokens.Git, "gitlab.com", "u", "t2")), tokens.Git)
	if ok, err := e.Forget(tokens.Git, "github.com", ""); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if h := hostsOf(e, tokens.Git); !slices.Equal(h, []string{"gitlab.com"}) {
		t.Fatal(h)
	}
	if ok, _ := e.Forget(tokens.Git, "github.com", ""); ok {
		t.Fatal("removed twice")
	}
	if _, err := e.Forget("bogus", "x", ""); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestForgetNPMDropsItsScopeLineButKeepsOthers(t *testing.T) {
	e, _ := sandbox(t)
	p := write(t, e, set(npm("npm.pkg.github.com", "tok", "@org")), tokens.NPM)
	if s := read(t, p); !strings.Contains(s, "@org:registry") || !strings.Contains(s, "_authToken") {
		t.Fatal(s)
	}
	_ = os.WriteFile(p, []byte(read(t, p)+"@other:registry=https://other.example/\n"), 0o600)
	if ok, _ := e.Forget(tokens.NPM, "npm.pkg.github.com", ""); !ok {
		t.Fatal("not removed")
	}
	s := read(t, p)
	if strings.Contains(s, "@org:registry") || strings.Contains(s, "_authToken") || !strings.Contains(s, "@other:registry=https://other.example/") {
		t.Fatal(s)
	}
}

func TestForgetComposerDropsTheEmptySection(t *testing.T) {
	e, _ := sandbox(t)
	write(t, e, set(gh("github.com", "x")), tokens.Composer)
	if ok, _ := e.Forget(tokens.Composer, "github.com", ""); !ok || len(hostsOf(e, tokens.Composer)) != 0 {
		t.Fatal(ok)
	}
}

func TestGitHostsWithAPathOrPortRoundTrip(t *testing.T) {
	e, _ := sandbox(t)
	write(t, e, set(basic(tokens.Git, "dev.azure.com/myorg", "me", "pat")), tokens.Git)
	if h := hostsOf(e, tokens.Git); !slices.Equal(h, []string{"dev.azure.com/myorg"}) {
		t.Fatal(h)
	}
	if ok, _ := e.Forget(tokens.Git, "https://dev.azure.com/myorg/", ""); !ok {
		t.Fatal("full-URL forget failed")
	}
	write(t, e, set(basic(tokens.Git, "git.acme:8443", "u", "P8443"), basic(tokens.Git, "git.acme", "u", "PLAIN")), tokens.Git)
	if h := hostsOf(e, tokens.Git); !slices.Equal(h, []string{"git.acme", "git.acme:8443"}) {
		t.Fatal(h)
	}
	if ok, _ := e.Forget(tokens.Git, "git.acme:8443", ""); !ok || !slices.Equal(hostsOf(e, tokens.Git), []string{"git.acme"}) {
		t.Fatal(hostsOf(e, tokens.Git))
	}
}

func TestGitInventorySurvivesAMalformedLine(t *testing.T) {
	e, _ := sandbox(t)
	p := write(t, e, set(basic(tokens.Git, "github.com", "me", "tok")), tokens.Git)
	_ = os.WriteFile(p, []byte(read(t, p)+"https://git.acme:8x43/foo\n"), 0o600)
	h := hostsOf(e, tokens.Git)
	if !slices.Contains(h, "github.com") || !slices.Contains(h, "git.acme:8x43/foo") {
		t.Fatal(h)
	}
	if ok, _ := e.Forget(tokens.Git, "git.acme:8x43/foo", ""); !ok || strings.Contains(read(t, p), "8x43") {
		t.Fatal(read(t, p))
	}
}

func put(t *testing.T, p, s string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryReadsLegacyAndSplitNPMAuth(t *testing.T) {
	e, _ := sandbox(t)
	p := e.TargetPath(tokens.NPM, "")
	put(t, p, "//legacy.example/:_auth="+base64.StdEncoding.EncodeToString([]byte("alice:secret"))+"\n")
	it := e.Inventory("", false)[0]
	if it.Host != "legacy.example" || it.Kind != "basic" || it.Username != "alice" || !it.SecretPresent {
		t.Fatalf("%+v", it)
	}
	pw := base64.StdEncoding.EncodeToString([]byte("sekret"))
	put(t, p, "//reg.example/:username=bob\n//reg.example/:_password="+pw+"\n//reg.example/:email=b@x.com\n")
	it = e.Inventory("", false)[0]
	if it.Host != "reg.example" || it.Username != "bob" || !it.SecretPresent {
		t.Fatalf("%+v", it)
	}
	if ok, _ := e.Forget(tokens.NPM, "reg.example", ""); !ok || strings.TrimSpace(read(t, p)) != "" {
		t.Fatal(read(t, p))
	}
}

func TestInventoryReadsADockerIdentityToken(t *testing.T) {
	e, _ := sandbox(t)
	put(t, e.TargetPath(tokens.Docker, ""), `{"auths": {"reg.example": {"identitytoken": "xyz"}}}`)
	it := e.Inventory("", false)[0]
	if it.Host != "reg.example" || !it.SecretPresent {
		t.Fatalf("%+v", it)
	}
}

func TestInventorySkipsMalformedFiles(t *testing.T) {
	e, _ := sandbox(t)
	put(t, e.TargetPath(tokens.Composer, ""), "{ not valid json")
	if len(e.Inventory("", false)) != 0 {
		t.Fatal("listed a malformed file")
	}
}

func TestNetrcEdgeCases(t *testing.T) {
	e, _ := sandbox(t)
	p := e.TargetPath(tokens.Netrc, "")
	put(t, p, "machine a.example login\nmachine b.example login me password pw\n")
	if h := hostsOf(e, tokens.Netrc); !slices.Equal(h, []string{"a.example", "b.example"}) {
		t.Fatal(h)
	}
	put(t, p, "machine a.com login u1 password p1 machine b.com login u2 password p2\n")
	if ok, _ := e.Forget(tokens.Netrc, "a.com", ""); !ok || !strings.Contains(read(t, p), "password p2") {
		t.Fatal(read(t, p))
	}
	if h := hostsOf(e, tokens.Netrc); !slices.Equal(h, []string{"b.com"}) {
		t.Fatal(h)
	}
	put(t, p, "machine a.com login u password p\nmacdef init\n  cd /pub\n\n")
	_, _ = e.Forget(tokens.Netrc, "a.com", "")
	if !strings.Contains(read(t, p), "macdef init") {
		t.Fatal(read(t, p))
	}
}

// Security fix 2: a malformed file is refused, not replaced (ATM replaced it
// with a warning, dropping whatever was in it).
func TestAMalformedTargetFileIsRefusedNotReplaced(t *testing.T) {
	e, _ := sandbox(t)
	p := e.TargetPath(tokens.Composer, "")
	put(t, p, "{ not json")
	if _, err := e.WriteTarget(set(gh("github.com", "x")), tokens.Composer, p); !errors.Is(err, tokens.ErrUnreadableJSON) {
		t.Fatal(err)
	}
	if read(t, p) != "{ not json" {
		t.Fatal("file changed")
	}
}

// test_aliases.py (file operations)

func TestInstallingAliasesBacksUpAndKeepsContent(t *testing.T) {
	e, dir := sandbox(t)
	rc := filepath.Join(dir, ".zshrc")
	_ = os.WriteFile(rc, []byte("export EXISTING=1\n"), 0o644)
	if _, _, err := e.Profile(rc, false).Install("authama", true); err != nil {
		t.Fatal(err)
	}
	s := read(t, rc)
	if !strings.Contains(s, "export EXISTING=1") || !tokens.HasAliasBlock(s) || !strings.Contains(s, "alias atm='authama token'") {
		t.Fatal(s)
	}
	if read(t, rc+AliasBackupSuffix) != "export EXISTING=1\n" {
		t.Fatal("no backup")
	}
	if removed, err := e.Profile(rc, false).Remove(false); !removed || err != nil {
		t.Fatal(removed, err)
	}
	if read(t, rc) != "export EXISTING=1\n" {
		t.Fatalf("%q", read(t, rc))
	}
	if removed, _ := e.Profile(rc, false).Remove(false); removed {
		t.Fatal("removed nothing twice")
	}
}

func TestInstallingAliasesKeepsTheFileMode(t *testing.T) {
	e, dir := sandbox(t)
	rc := filepath.Join(dir, ".zshrc")
	_ = os.WriteFile(rc, []byte("x\n"), 0o644)
	_ = os.Chmod(rc, 0o644)
	fi, _ := os.Stat(rc)
	before := fi.Mode().Perm()
	_, _, _ = e.Profile(rc, false).Install("authama", false)
	fi, _ = os.Stat(rc)
	if fi.Mode().Perm() != before {
		t.Fatalf("%o -> %o", before, fi.Mode().Perm())
	}
}

func TestAliasStatusListsOnlyManagedAliases(t *testing.T) {
	e, dir := sandbox(t)
	rc := filepath.Join(dir, ".zshrc")
	_ = os.WriteFile(rc, []byte("alias ll='ls -l'\n"), 0o644)
	if st := e.Profile(rc, false).Status(); st.Present {
		t.Fatal("present before install")
	}
	_, _, _ = e.Profile(rc, false).Install("authama", false)
	st := e.Profile(rc, false).Status()
	if !st.Present || len(st.Lines) != 4 || strings.Contains(strings.Join(st.Lines, "\n"), "ll=") {
		t.Fatalf("%+v", st)
	}
}

func TestInstallingAliasesReplacesTheATMBlock(t *testing.T) {
	e, dir := sandbox(t)
	rc := filepath.Join(dir, ".bashrc")
	_ = os.WriteFile(rc, []byte("a\n# >>> auth-token-manager >>>\nalias atm='auth-token-manager'\n# <<< auth-token-manager <<<\n"), 0o644)
	updated, _, err := e.Profile(rc, false).Install("authama", false)
	if err != nil || !updated {
		t.Fatal(updated, err)
	}
	s := read(t, rc)
	if strings.Contains(s, "auth-token-manager") || strings.Count(s, tokens.AliasBegin) != 1 {
		t.Fatal(s)
	}
}

func TestTheDefaultProfileFollowsTheShell(t *testing.T) {
	e, dir := sandbox(t, "SHELL", "/bin/zsh")
	if p := e.Profile("", false).File; p != filepath.Join(dir, ".zshrc") {
		t.Fatal(p)
	}
	if p := e.Profile("", true).File; p != filepath.Join(dir, ".zprofile") {
		t.Fatal(p)
	}
}

// Config discovery and adoption.

func TestConfigDiscoveryOrder(t *testing.T) {
	e, dir := sandbox(t)
	if src, err := e.FindConfig(""); err != nil || src.Path != "" {
		t.Fatal(src, err)
	}
	legacy := filepath.Join(dir, ".config", LegacyConfigName)
	put(t, legacy, `{"npm":{}}`)
	if src, _ := e.FindConfig(""); src.Path != legacy || !src.Legacy {
		t.Fatal(src)
	}
	put(t, e.Paths.Tokens(), `{}`)
	if src, _ := e.FindConfig(""); src.Path != e.Paths.Tokens() || src.Legacy {
		t.Fatal(src)
	}
	local := filepath.Join(dir, LocalConfigName)
	put(t, local, `{}`)
	if src, _ := e.FindConfig(""); src.Path != local {
		t.Fatal(src)
	}
	if _, err := e.FindConfig(filepath.Join(dir, "nope.json")); err == nil {
		t.Fatal("missing --config accepted")
	}
}

func TestImportCopiesOnceAndKeepsTheOriginal(t *testing.T) {
	e, dir := sandbox(t)
	if _, _, err := e.Import("", false); !errors.Is(err, ErrNoLegacyConfig) {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, LegacyConfigName)
	put(t, legacy, `{"npm":{"registry.npmjs.org":{"env":"NPM_TOKEN"}}}`)
	from, to, err := e.Import("", false)
	if err != nil || from != legacy || to != e.Paths.Tokens() {
		t.Fatal(from, to, err)
	}
	if !strings.Contains(read(t, to), "NPM_TOKEN") || !exists(legacy) {
		t.Fatal("not adopted")
	}
	if _, _, err := e.Import("", false); err == nil {
		t.Fatal("overwrote tokens.json")
	}
}

// Docker credential stores (security fix 1).

func TestDockerHostsInACredentialStoreAreRefusedOrLoggedIn(t *testing.T) {
	e, _ := sandbox(t)
	p := e.TargetPath(tokens.Docker, "")
	put(t, p, `{"credsStore": "desktop", "credHelpers": {"gcr.io": "gcloud"}}`)
	s := set(basic(tokens.Docker, "ghcr.io", "u", "pw"))
	if !HasDockerDelegation(s, p) {
		t.Fatal("delegation missed")
	}
	res, err := e.SyncDocker(s, p, DockerRefuse)
	if err != nil || !slices.Equal(res.Skipped, []string{"ghcr.io"}) || strings.Contains(read(t, p), "auths") {
		t.Fatalf("%+v %v", res, err)
	}
	var calls []string
	orig := DockerLoginRunner
	DockerLoginRunner = func(dir, host, user, pass string) error {
		calls = append(calls, dir+"|"+host+"|"+user+"|"+pass)
		return nil
	}
	t.Cleanup(func() { DockerLoginRunner = orig })
	res, err = e.SyncDocker(s, p, DockerLogin)
	if err != nil || !slices.Equal(calls, []string{filepath.Dir(p) + "|ghcr.io|u|pw"}) || len(res.LoggedIn) != 1 {
		t.Fatalf("%+v %v %v", res, err, calls)
	}
}

func TestDockerHostsWithoutAStoreAreWrittenInline(t *testing.T) {
	e, _ := sandbox(t)
	p := e.TargetPath(tokens.Docker, "")
	put(t, p, `{"credHelpers": {"gcr.io": "gcloud"}}`)
	res, err := e.SyncDocker(set(basic(tokens.Docker, "ghcr.io", "u", "pw")), p, DockerRefuse)
	if err != nil || !slices.Equal(res.Written, []string{"ghcr.io"}) || !strings.Contains(read(t, p), `"ghcr.io"`) {
		t.Fatalf("%+v %v", res, err)
	}
}

// Dev mode (security fix 5).

func devEnv(t *testing.T, vars ...string) (Env, string) {
	t.Helper()
	root := t.TempDir()
	env := map[string]string{"HOME": t.TempDir()}
	for i := 0; i+1 < len(vars); i += 2 {
		env[vars[i]] = vars[i+1]
	}
	get := func(k string) string { return env[k] }
	p, err := paths.ResolveDev(get, root, root)
	if err != nil {
		t.Fatal(err)
	}
	return NewEnv(p, get, func(k string) (string, bool) { v, ok := env[k]; return v, ok }, root), root
}

func TestDevModeResolvesEveryTargetInsideTheSandbox(t *testing.T) {
	outside := t.TempDir()
	e, root := devEnv(t, "DOCKER_CONFIG", outside, "NETRC", filepath.Join(outside, "n"),
		"NPM_CONFIG_USERCONFIG", filepath.Join(outside, "npmrc"), "COMPOSER_HOME", outside)
	for _, tg := range tokens.Targets() {
		p := e.TargetPath(tg, "")
		if !strings.HasPrefix(p, filepath.Join(root, paths.DevHomeDir)) {
			t.Fatalf("%s -> %s", tg, p)
		}
		if err := e.Guard(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.WriteTarget(set(gh("github.com", "x")), tokens.Composer, filepath.Join(outside, "auth.json")); !errors.Is(err, ErrOutsideSandbox) {
		t.Fatal(err)
	}
	if exists(filepath.Join(outside, "auth.json")) {
		t.Fatal("wrote outside the sandbox")
	}
}

func TestDevModeRefusesSymlinkEscapes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges")
	}
	outside := t.TempDir()
	e, root := devEnv(t)
	home := filepath.Join(root, paths.DevHomeDir)
	_ = os.MkdirAll(home, 0o700)
	if err := os.Symlink(outside, filepath.Join(home, ".docker")); err != nil {
		t.Fatal(err)
	}
	if err := e.Guard(e.TargetPath(tokens.Docker, "")); !errors.Is(err, ErrOutsideSandbox) {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "x"), filepath.Join(home, ".netrc")); err != nil {
		t.Fatal(err)
	}
	if err := e.Guard(e.TargetPath(tokens.Netrc, "")); err == nil {
		t.Fatal("wrote through a symlink")
	}
}

func TestDevModeRefusesDockerLogin(t *testing.T) {
	e, _ := devEnv(t)
	p := e.TargetPath(tokens.Docker, "")
	put(t, p, `{"credsStore": "desktop"}`)
	orig := DockerLoginRunner
	DockerLoginRunner = func(string, string, string, string) error { t.Fatal("ran docker"); return nil }
	t.Cleanup(func() { DockerLoginRunner = orig })
	if _, err := e.SyncDocker(set(basic(tokens.Docker, "ghcr.io", "u", "pw")), p, DockerLogin); err == nil {
		t.Fatal("docker login ran in dev mode")
	}
}

func TestImportANamedFileWithADryRunFirst(t *testing.T) {
	e, dir := sandbox(t)
	team := filepath.Join(dir, "team.json")
	put(t, team, `{"git":{"github.com":{"user_env":"U","pass_env":"P"}}}`)
	if _, to, err := e.Import(team, true); err != nil || exists(to) {
		t.Fatalf("dry run: %v, wrote %v", err, exists(to))
	}
	if _, _, err := e.Import(filepath.Join(dir, "absent.json"), false); err == nil {
		t.Fatal("a missing file was imported")
	}
	from, to, err := e.Import(team, false)
	if err != nil || from != team || !strings.Contains(read(t, to), "pass_env") {
		t.Fatal(from, to, err)
	}
	if _, _, err := e.Import(to, false); err == nil {
		t.Fatal("imported tokens.json onto itself")
	}
}

// test_windows.py: is_owner_only is true on Windows (ACLs, not mode bits;
// SetPerms enforces them with icacls on every write) and the perms column
// reads as text, never a crash.
func TestOwnerOnlyAndPermsOnWindows(t *testing.T) {
	f := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(f, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ownerOnlyOn(true, f) {
		t.Error("an existing file should pass the Windows check")
	}
	if ownerOnlyOn(false, f) {
		t.Error("a 0644 file should fail the POSIX check")
	}
	if got := permStringOn(true, f); got != "ACL" {
		t.Errorf("windows perms = %q, want ACL", got)
	}
	if got := permStringOn(true, f+".absent"); got != "absent" {
		t.Errorf("absent file perms = %q", got)
	}
	if ownerOnlyOn(true, f+".absent") {
		t.Error("an absent file must not pass")
	}
}
