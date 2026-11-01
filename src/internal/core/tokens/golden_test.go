package tokens

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// golden.json is generated from the Python auth-token-manager by
// testdata/gen_golden.py. Every renderer, merge, scanner, purge, key and
// export here must reproduce it byte for byte, so files either tool wrote stay
// readable and editable by the other.

type goldenCred struct {
	Target     string   `json:"target"`
	Host       string   `json:"host"`
	Kind       string   `json:"kind"`
	Mechanism  string   `json:"mechanism"`
	Token      string   `json:"token"`
	Username   string   `json:"username"`
	Password   string   `json:"password"`
	Key        string   `json:"key"`
	Secret     string   `json:"secret"`
	Headers    []string `json:"headers"`
	Scope      string   `json:"scope"`
	AlwaysAuth bool     `json:"always_auth"`
}

type golden struct {
	Renders []struct {
		Set      string   `json:"set"`
		Target   string   `json:"target"`
		Existing *string  `json:"existing"`
		Out      string   `json:"out"`
		Warnings []string `json:"warnings"`
	} `json:"renders"`
	Scans []struct {
		Target string      `json:"target"`
		Text   string      `json:"text"`
		Out    []Installed `json:"out"`
	} `json:"scans"`
	Purges []struct {
		Target, Host, Text, Out string
	} `json:"purges"`
	Misc struct {
		GitKey    map[string]string `json:"gitkey"`
		Quote     map[string]string `json:"quote"`
		EnvExport []struct {
			Shell, Value, Out string
		} `json:"env_export"`
		Describe []struct {
			Target, Host, Out string
		} `json:"describe"`
		AliasBlock string `json:"alias_block"`
		StripBlock []struct {
			Text, Install, Remove string
		} `json:"strip_block"`
	} `json:"misc"`
	Configs struct {
		Env   map[string]string `json:"env"`
		Cases []struct {
			Doc      json.RawMessage   `json:"doc"`
			Warnings []string          `json:"warnings"`
			Renders  map[string]string `json:"renders"`
		} `json:"cases"`
	} `json:"configs"`
}

// goldenSets mirrors SETS in gen_golden.py.
var goldenSets = map[string][]goldenCred{}

func loadGolden(t *testing.T) golden {
	t.Helper()
	b, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatal(err)
	}
	sets, err := os.ReadFile("testdata/golden_sets.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sets, &goldenSets); err != nil {
		t.Fatal(err)
	}
	return g
}

func buildSet(name string) *Set {
	if name == "template" {
		return Template()
	}
	s := NewSet()
	for _, c := range goldenSets[name] {
		s.Add(Credential(c))
	}
	return s
}

func TestRendersMatchThePythonTool(t *testing.T) {
	g := loadGolden(t)
	if len(g.Renders) == 0 {
		t.Fatal("no render cases")
	}
	for i, c := range g.Renders {
		s := buildSet(c.Set)
		var got string
		switch c.Target {
		case "_json":
			got = s.RedactedJSON(nil)
		case "_composer_env":
			got = s.ComposerEnv(ShellPOSIX)
		default:
			var err error
			existing, present := "", c.Existing != nil
			if present {
				existing = *c.Existing
			}
			got, err = s.Render(c.Target, existing, present)
			if err != nil {
				got = "ERROR: " + err.Error()
			}
		}
		if got != c.Out {
			t.Errorf("case %d (%s/%s, existing %q):\n got: %q\nwant: %q", i, c.Set, c.Target, deref(c.Existing), got, c.Out)
		}
	}
}

func deref(p *string) string {
	if p == nil {
		return "<absent>"
	}
	return *p
}

func TestScansMatchThePythonTool(t *testing.T) {
	g := loadGolden(t)
	for i, c := range g.Scans {
		got := Scan(c.Target, c.Text)
		for j := range got {
			got[j].Description = ""
		}
		if len(got) == 0 && len(c.Out) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, c.Out) {
			t.Errorf("scan %d (%s %q):\n got: %+v\nwant: %+v", i, c.Target, c.Text, got, c.Out)
		}
	}
}

func TestPurgesMatchThePythonTool(t *testing.T) {
	g := loadGolden(t)
	for i, c := range g.Purges {
		if got := Purge(c.Target, c.Host, c.Text); got != c.Out {
			t.Errorf("purge %d (%s %s from %q):\n got: %q\nwant: %q", i, c.Target, c.Host, c.Text, got, c.Out)
		}
	}
}

func TestKeysQuotesExportsAndLabelsMatchThePythonTool(t *testing.T) {
	g := loadGolden(t)
	for in, want := range g.Misc.GitKey {
		if got := GitKey(in); got != want {
			t.Errorf("GitKey(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range g.Misc.Quote {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
	for _, c := range g.Misc.EnvExport {
		if got := EnvExport("X", c.Value, c.Shell); got != c.Out {
			t.Errorf("EnvExport(%s, %q) = %q, want %q", c.Shell, c.Value, got, c.Out)
		}
	}
	for _, c := range g.Misc.Describe {
		if got := Describe(c.Target, c.Host); got != c.Out {
			t.Errorf("Describe(%s, %s) = %q, want %q", c.Target, c.Host, got, c.Out)
		}
	}
	for _, c := range g.Misc.StripBlock {
		if got := RenderAliasInstall(c.Text, "BLOCK"); got != c.Install {
			t.Errorf("install into %q = %q, want %q", c.Text, got, c.Install)
		}
		if got := RenderAliasRemove(c.Text); got != c.Remove {
			t.Errorf("remove from %q = %q, want %q", c.Text, got, c.Remove)
		}
	}
}

func TestConfigResolutionMatchesThePythonTool(t *testing.T) {
	g := loadGolden(t)
	get := func(k string) (string, bool) {
		v, ok := g.Configs.Env[k]
		return v, ok
	}
	for i, c := range g.Configs.Cases {
		cfg, err := ParseConfig(string(c.Doc))
		if err != nil {
			t.Fatal(err)
		}
		s := cfg.Resolve(get)
		if !reflect.DeepEqual(s.Warnings, c.Warnings) && !(len(s.Warnings) == 0 && len(c.Warnings) == 0) {
			t.Errorf("config %d warnings:\n got: %q\nwant: %q", i, s.Warnings, c.Warnings)
		}
		got := map[string]string{}
		for _, tgt := range s.Targets() {
			got[tgt], _ = s.Render(tgt, "", false)
		}
		if !reflect.DeepEqual(got, c.Renders) && !(len(got) == 0 && len(c.Renders) == 0) {
			t.Errorf("config %d renders:\n got: %q\nwant: %q", i, got, c.Renders)
		}
	}
}
