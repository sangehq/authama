package highlight

import (
	"strings"
	"testing"
)

func join(ts []Token) string {
	var b strings.Builder
	for _, t := range ts {
		b.WriteString(t.Text)
	}
	return b.String()
}

func kinds(ts []Token) map[string]Kind {
	m := map[string]Kind{}
	for _, t := range ts {
		m[strings.TrimSpace(t.Text)] = t.Kind
	}
	return m
}

// Whatever the language, the tokens put the line back together exactly: the
// highlighter can colour a line wrongly, never change it.
func TestTokensRebuildTheLine(t *testing.T) {
	lines := []string{
		`  "$schema": "https://example/x.json",`, `{"a": [1, -2.5e3, true, null], "b\"c": "d"}`, `"unterminated`,
		`[pypi]`, `username = __token__`, `; comment`, `key: 'v'`,
		`[registries.acme]`, `token = "x"`,
		`- name: web`, `"https://g.example": abc`, `---`,
		`Host gh github`, `  HostName github.com`, `  Port 22`, `# c`,
		`machine api.example login me password p`,
		`export FOO="bar baz"`, `A=1`, ``, "\t\t", `中文 = 値`,
	}
	for _, lang := range []Lang{Plain, JSON, INI, TOML, YAML, Properties, SSHConfig, Netrc, Shell} {
		for _, l := range lines {
			if got := join(Line(lang, l)); got != l {
				t.Errorf("%s %q rebuilt as %q", lang, l, got)
			}
		}
	}
}

func TestEachLanguageFindsItsParts(t *testing.T) {
	for _, tc := range []struct {
		lang Lang
		line string
		want map[string]Kind
	}{
		{JSON, `  "version": 1,`, map[string]Kind{`"version"`: Key, "1": Number, ":": Punct}},
		{JSON, `  "on": true`, map[string]Kind{`"on"`: Key, "true": Literal}},
		{JSON, `  "x": "y"`, map[string]Kind{`"y"`: String}},
		{INI, `[pypi]`, map[string]Kind{"[pypi]": Section}},
		{INI, `username = __token__`, map[string]Kind{"username": Key, "__token__": String}},
		{INI, `# note`, map[string]Kind{"# note": Comment}},
		{YAML, `:rubygems_api_key: abc`, map[string]Kind{":rubygems_api_key": Key, "abc": String}},
		{SSHConfig, `Host gh`, map[string]Kind{"Host": Section, "gh": String}},
		{SSHConfig, `  Port 22`, map[string]Kind{"Port": Key, "22": Number}},
		{Netrc, `machine h login u`, map[string]Kind{"machine": Section, "h": String, "login": Key, "u": String}},
		{Shell, `export A=1`, map[string]Kind{"export": Section, "A": Key, "1": Number}},
	} {
		got := kinds(Line(tc.lang, tc.line))
		for text, k := range tc.want {
			if got[text] != k {
				t.Errorf("%s %q: %q is %q, want %q (%v)", tc.lang, tc.line, text, got[text], k, got)
			}
		}
	}
}

func TestForNamesTheLanguageFromThePath(t *testing.T) {
	for path, want := range map[string]Lang{
		"config/authama.json": JSON, "~/.docker/config.json": JSON, "/h/.pypirc": INI,
		"/h/.cargo/credentials.toml": TOML, "/h/.gem/credentials": YAML, "/h/.gradle/gradle.properties": Properties,
		"/h/.ssh/config": SSHConfig, "/h/.netrc": Netrc, "/h/_netrc": Netrc, "config/.env": Shell, "notes.txt": Plain,
	} {
		if got := For(path); got != want {
			t.Errorf("For(%q) = %q, want %q", path, got, want)
		}
	}
}
