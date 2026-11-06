package tokens

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/testcred"
)

// Each added tool's file round-trips: merging a credential into a file that
// already holds other things keeps every one of them, Scan reads the
// credential back, merging again replaces rather than duplicates, and Purge
// removes it and nothing else.
func TestTheAddedFormatsRoundTrip(t *testing.T) {
	user := "__token__"
	for _, tc := range []struct {
		target, host, existing string
		keep                   []string // lines from existing that must survive
		want                   string   // a line the merge must write
	}{
		{PyPI, "upload.pypi.org", "[distutils]\nindex-servers =\n    private\n\n[private]\nrepository = https://pkgs.example/\nusername = alice\npassword = %s\n",
			[]string{"[private]", "repository = https://pkgs.example/", "username = alice"}, "[pypi]"},
		{PyPI, "test.pypi.org", "", nil, "repository = https://test.pypi.org/legacy/"},
		{Cargo, "crates.io", "[registries.other]\ntoken = \"%s\"\n\n[unrelated]\nkey = 1\n",
			[]string{"[registries.other]", "[unrelated]", "key = 1"}, "[registry]"},
		{Cargo, "acme", "", nil, "[registries.acme]"},
		{Gem, "rubygems.org", "---\n\"https://gems.example\": %s\n",
			[]string{`"https://gems.example": `}, ":rubygems_api_key: "},
		{Gem, "rubygems.pkg.github.com/acme", "", nil, `"https://rubygems.pkg.github.com/acme": `},
		{Gradle, "acme", "org.gradle.jvmargs=-Xmx2g\notherUsername=bob\notherPassword=%s\n",
			[]string{"org.gradle.jvmargs=-Xmx2g", "otherUsername=bob"}, "acmeUsername=__token__"},
	} {
		t.Run(tc.target+" "+tc.host, func(t *testing.T) {
			secret := testcred.Secret(tc.target)
			existing := tc.existing
			if strings.Contains(existing, "%s") {
				existing = strings.ReplaceAll(existing, "%s", testcred.Secret("other"))
			}
			set := NewSet()
			cr := Credential{Username: user, Password: secret, Token: secret}
			if !set.Apply(tc.target, "", tc.host, cr) {
				t.Fatalf("Apply refused %s %s", tc.target, tc.host)
			}
			out, err := set.Render(tc.target, existing, existing != "")
			if err != nil {
				t.Fatal(err)
			}
			for _, k := range append(tc.keep, tc.want) {
				if !strings.Contains(out, k) {
					t.Errorf("rendered file lost or lacks %q:\n%s", k, out)
				}
			}
			found := false
			for _, it := range Scan(tc.target, out) {
				if it.Host == CanonicalHost(tc.target, tc.host) || (tc.target == PyPI && it.Host == tc.host) {
					found = it.Secret == secret
				}
			}
			if !found {
				t.Errorf("Scan did not read the credential back: %+v\n%s", Scan(tc.target, out), out)
			}
			again, err := set.Render(tc.target, out, true)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(again, secret) != 1 {
				t.Errorf("merging twice wrote the secret %d times:\n%s", strings.Count(again, secret), again)
			}
			purged := Purge(tc.target, tc.host, again)
			if strings.Contains(purged, secret) {
				t.Errorf("Purge left the credential:\n%s", purged)
			}
			for _, k := range tc.keep {
				if !strings.Contains(purged, k) {
					t.Errorf("Purge removed %q, which was not ours:\n%s", k, purged)
				}
			}
		})
	}
}

// .pypirc's index-servers lists exactly the sections there are, after a
// merge and after a purge, without duplicating the list.
func TestPypircKeepsIndexServersInStep(t *testing.T) {
	set := NewSet()
	set.Apply(PyPI, "", "upload.pypi.org", Credential{Username: "__token__", Password: testcred.Secret("pypi")})
	set.Apply(PyPI, "", "test.pypi.org", Credential{Username: "__token__", Password: testcred.Secret("testpypi")})
	out, err := set.Render(PyPI, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "index-servers") != 1 || !strings.Contains(out, "    pypi\n    testpypi") {
		t.Fatalf("index-servers:\n%s", out)
	}
	purged := Purge(PyPI, "test.pypi.org", out)
	if strings.Contains(purged, "testpypi") || !strings.Contains(purged, "    pypi") {
		t.Errorf("after purging testpypi:\n%s", purged)
	}
}

// A host or value a format cannot hold safely is refused before it is written.
func TestTheAddedFormatsRefuseWhatTheyCannotHold(t *testing.T) {
	for _, c := range []Credential{
		{Target: Cargo, Host: "not a name", Kind: KindToken, Token: testcred.Secret("c")},
		{Target: Gradle, Host: "9lives", Kind: KindBasic, Username: "u", Password: testcred.Secret("g")},
		{Target: PyPI, Host: "[evil]", Kind: KindBasic, Username: "u", Password: testcred.Secret("p")},
		{Target: PyPI, Host: "pkgs.example", Kind: KindBasic, Username: "two words", Password: testcred.Secret("p")},
	} {
		if err := Check(c); err == nil {
			t.Errorf("%s %q passed", c.Target, c.Host)
		}
	}
	for tgt, want := range map[string]string{PyPI: KindBasic, Cargo: KindToken, Gem: KindToken, Gradle: KindBasic, NPM: KindToken, Git: KindBasic, Composer: ""} {
		if got := TargetKind(tgt); got != want {
			t.Errorf("TargetKind(%s) = %q, want %q", tgt, got, want)
		}
	}
}

// Every variable a format reads to find its file is one dev mode hides, so a
// sandboxed run can never be redirected at a real file.
func TestEveryPathVariableAFormatReadsIsHiddenInDevMode(t *testing.T) {
	hidden := map[string]bool{}
	for _, v := range PathOverrideVars {
		hidden[v] = true
	}
	for _, v := range []string{"CARGO_HOME", "GRADLE_USER_HOME"} {
		if !hidden[v] {
			t.Errorf("%s redirects a credential file and is not in PathOverrideVars", v)
		}
	}
	// And the formats read them: a sandbox host with them set elsewhere still
	// resolves inside its own home only when they are hidden.
	h := Host{Home: "/sandbox/home", Getenv: func(k string) string {
		if k == "CARGO_HOME" || k == "GRADLE_USER_HOME" {
			return "/real/place"
		}
		return ""
	}}
	if got := h.TargetPath(Cargo, ""); got != filepath.Join("/real/place", "credentials.toml") {
		t.Errorf("cargo does not read CARGO_HOME: %s", got)
	}
}

// A config entry for each added tool resolves from the environment like the
// first five did; until this was fixed, apply skipped every one as an
// "unknown target".
func TestTheAddedToolsResolveFromTheEnvironment(t *testing.T) {
	cfg, err := ParseConfig(`{
	  "pypi":   {"upload.pypi.org": {"user_env": "PYPI_USER", "pass_env": "PYPI_TOKEN"}},
	  "cargo":  {"crates.io": {"env": "CARGO_TOKEN"}},
	  "gem":    {"rubygems.org": {"env": "GEM_KEY"}},
	  "gradle": {"acme": {"user_env": "ACME_USER", "pass_env": "ACME_PASS"}}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"PYPI_USER": "__token__", "PYPI_TOKEN": testcred.Secret("pypi"), "CARGO_TOKEN": testcred.Secret("cargo"),
		"GEM_KEY": testcred.Secret("gem"), "ACME_USER": "bob", "ACME_PASS": testcred.Secret("gradle"),
	}
	set := cfg.Resolve(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	for _, w := range set.Warnings {
		if strings.Contains(w, "unknown target") {
			t.Errorf("warning: %s", w)
		}
	}
	for _, tgt := range []string{PyPI, Cargo, Gem, Gradle} {
		out, err := set.Render(tgt, "", false)
		if err != nil || out == "" {
			t.Errorf("%s rendered nothing: %v", tgt, err)
		}
	}
}
