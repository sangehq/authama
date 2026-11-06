package naming

import (
	"errors"
	"strings"
	"testing"
)

func TestTemplatesAreChecked(t *testing.T) {
	for _, ok := range []string{DefaultAlias, "{code}.{env}.{role}.{nn}", "{env}-{role}-{nn}", "srv-{code}-{nn}"} {
		if err := CheckAlias(ok); err != nil {
			t.Errorf("CheckAlias(%q) = %v", ok, err)
		}
	}
	for bad, why := range map[string]string{
		"plain":                     "names no part",
		"{code}-{env}-{role}":       "no number",
		"{code}{env}-{nn}":          "touch",
		"{code}-{zone}-{nn}":        "a part is not",
		"{code}_{env}-{nn}":         "host name",
		"{code}-{code}-{nn}":        "twice",
		"{code}-{env}-{role}-{nn}/": "host name",
		"{Code}-{env}-{role}-{nn}":  "",
	} {
		err := CheckAlias(bad)
		if err == nil || (why != "" && !strings.Contains(err.Error(), why)) {
			t.Errorf("CheckAlias(%q) = %v, want %q", bad, err, why)
		}
	}
	for _, ok := range []string{DefaultKeyName, "{profile}_{code}_{env}_{role}_{nn}-{algo}", "{profile}_{alias}-key-{algo}"} {
		if err := CheckKeyName(ok); err != nil {
			t.Errorf("CheckKeyName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"{alias}-{algo}", "{profile}_{alias}", "{profile}_fixed-{algo}", "{profile}_{alias}-{when}-{algo}", "{profile}_a/{alias}-{algo}"} {
		if CheckKeyName(bad) == nil {
			t.Errorf("CheckKeyName(%q) was accepted", bad)
		}
	}
	if (Rules{Enforce: "loud"}).Check() == nil || (Rules{Envs: []string{"Prod"}}).Check() == nil {
		t.Error("a bad level or env was accepted")
	}
}

func TestAnAliasIsParsedAndChecked(t *testing.T) {
	r := Rules{Envs: []string{"prd", "stg", "dev"}}
	p, ok := r.Parse("simbi-prd-web-00")
	if !ok || p["code"] != "simbi" || p["env"] != "prd" || p["role"] != "web" || p["nn"] != "00" {
		t.Fatalf("Parse = %v %v", p, ok)
	}
	for alias, want := range map[string]string{
		"simbi-prd-web-00": "",
		"oribi-web":        "does not follow",
		"dani-prd-web-00":  "its profile's code is simbi",
		"simbi-qa-web-01":  "env qa is not one of",
	} {
		got := r.Violation(alias, "simbi")
		if (want == "" && got != "") || (want != "" && !strings.Contains(got, want)) {
			t.Errorf("Violation(%q) = %q, want %q", alias, got, want)
		}
	}
}

// The next server of a kind takes the lowest free number; servers of another
// kind, or named some other way, do not count.
func TestTheNextAliasTakesTheLowestFreeNumber(t *testing.T) {
	r := Rules{}
	taken := []string{"simbi-prd-web-00", "simbi-prd-web-02", "simbi-prd-db-00", "simbi-stg-web-01", "dani-prd-web-01", "oribi-web"}
	got, err := r.Generate(map[string]string{"code": "simbi", "env": "prd", "role": "web"}, taken)
	if err != nil || got != "simbi-prd-web-01" {
		t.Fatalf("Generate = %q, %v", got, err)
	}
	got, _ = r.Generate(map[string]string{"code": "simbi", "env": "prd", "role": "cache"}, taken)
	if got != "simbi-prd-cache-00" {
		t.Errorf("a new role starts at 00: %q", got)
	}
	if _, err := r.Generate(map[string]string{"code": "simbi", "env": "prd"}, nil); err == nil {
		t.Error("a missing role was accepted")
	}
	if _, err := (Rules{Roles: []string{"web", "db"}}).Generate(map[string]string{"code": "simbi", "env": "prd", "role": "mail"}, nil); err == nil {
		t.Error("a role outside the list was accepted")
	}
	if _, err := r.Generate(map[string]string{"code": "Simbi", "env": "prd", "role": "web"}, nil); err == nil {
		t.Error("an uppercase code was accepted")
	}
}

func TestTheKeyNameTemplate(t *testing.T) {
	def := Rules{}
	if got, _ := def.RenderKeyName("simtabi", "simbi-prd-web-00", "simbi", "ed25519"); got != "simtabi_simbi-prd-web-00-ed25519" {
		t.Errorf("default = %q", got)
	}
	parts := Rules{KeyName: "{profile}_{code}_{env}_{role}_{nn}-{algo}"}
	if got, _ := parts.RenderKeyName("simtabi", "simbi-prd-web-00", "simbi", "ed25519"); got != "simtabi_simbi_prd_web_00-ed25519" {
		t.Errorf("parts = %q", got)
	}
	if _, err := parts.RenderKeyName("simtabi", "oribi-web", "simbi", "ed25519"); !errors.Is(err, ErrNoParts) {
		t.Errorf("an alias without parts: %v, want ErrNoParts", err)
	}
	// {code} comes from the profile when it has one, so a template naming only
	// {code} works for any alias.
	if got, err := (Rules{KeyName: "{profile}_{code}-{alias}-{algo}"}).RenderKeyName("simtabi", "oribi-web", "simbi", "rsa"); err != nil ||
		got != "simtabi_simbi-oribi-web-rsa" {
		t.Errorf("code from the profile = %q, %v", got, err)
	}
}

// An optional part is left out, with its separator, when it has no value, and
// an alias without it still parses.
func TestAnOptionalPartIsLeftOutWithItsSeparator(t *testing.T) {
	cases := []struct {
		tpl, code, env, want string
	}{
		{"{code?}-{env}-{role}-{nn}", "simbi", "prd", "simbi-prd-web-00"},
		{"{code?}-{env}-{role}-{nn}", "", "prd", "prd-web-00"},
		{"{code}-{env?}-{role}-{nn}", "simbi", "prd", "simbi-prd-web-00"},
		{"{code}-{env?}-{role}-{nn}", "simbi", "", "simbi-web-00"},
	}
	for _, c := range cases {
		r := Rules{Alias: c.tpl}
		got, err := r.Generate(map[string]string{"code": c.code, "env": c.env, "role": "web"}, nil)
		if err != nil || got != c.want {
			t.Errorf("%s: Generate = %q, %v; want %q", c.tpl, got, err, c.want)
		}
		p, ok := r.Parse(c.want)
		if !ok || p["role"] != "web" || p["nn"] != "00" || p["code"] != c.code || p["env"] != c.env {
			t.Errorf("%s: Parse(%q) = %v %v", c.tpl, c.want, p, ok)
		}
	}
	if CheckAlias("{code}-{nn?}") == nil {
		t.Error("an optional number was accepted")
	}
	if err := CheckAlias("{code?}-{env?}-{role}-{nn}"); err == nil || !strings.Contains(err.Error(), "in a row") {
		t.Errorf("two optional parts in a row: %v", err)
	}
}

// Listed envs and roles make reading an alias exact: an env that is also a
// plausible code is read as the env it is.
func TestListedValuesMakeParsingExact(t *testing.T) {
	r := Rules{Alias: "{code}-{env}-{role}-{nn}", Envs: []string{"prd", "stg"}}
	if v := r.Violation("simbi-qa-web-00", "simbi"); !strings.Contains(v, "env qa is not one of") {
		t.Errorf("an unlisted env: %q", v)
	}
	if p, ok := r.Parse("simbi-stg-web-01"); !ok || p["env"] != "stg" {
		t.Errorf("Parse = %v %v", p, ok)
	}
}

// The number can have any width and start at 1.
func TestTheNumberHasAWidthAndAStart(t *testing.T) {
	r := Rules{Alias: "{code}-{role}-{nnn}", Start: 1}
	got, _ := r.Generate(map[string]string{"code": "simbi", "role": "web"}, []string{"simbi-web-001"})
	if got != "simbi-web-002" {
		t.Errorf("width 3 from 1 = %q", got)
	}
	got, _ = r.Generate(map[string]string{"code": "simbi", "role": "db"}, nil)
	if got != "simbi-db-001" {
		t.Errorf("first from 1 = %q", got)
	}
	if (Rules{Start: 2}).Check() == nil {
		t.Error("start 2 was accepted")
	}
}

// A profile with no code has none, unless the fallback is its name.
func TestAProfileWithoutACode(t *testing.T) {
	if got := (Rules{}).Code("Acme-Ltd", ""); got != "" {
		t.Errorf("no fallback = %q", got)
	}
	if got := (Rules{CodeFallback: CodeProfile}).Code("Acme-Ltd", ""); got != "acmeltd" {
		t.Errorf("profile fallback = %q", got)
	}
	if got := (Rules{CodeFallback: CodeProfile}).Code("simtabi", "simbi"); got != "simbi" {
		t.Errorf("an own code loses to the fallback: %q", got)
	}
	if _, err := (Rules{}).Generate(map[string]string{"env": "prd", "role": "web"}, nil); err == nil ||
		!strings.Contains(err.Error(), "no code") {
		t.Errorf("a required code with none: %v", err)
	}
	// A key template with an optional code names keys either way.
	r := Rules{KeyName: "{profile}_{code?}_{alias}-{algo}"}
	if got, _ := r.RenderKeyName("simtabi", "web-00", "simbi", "ed25519"); got != "simtabi_simbi_web-00-ed25519" {
		t.Errorf("with a code = %q", got)
	}
	if got, _ := r.RenderKeyName("work", "web-00", "", "ed25519"); got != "work_web-00-ed25519" {
		t.Errorf("without = %q", got)
	}
	if CheckKeyName("{profile}_{code?}-{algo}") == nil {
		t.Error("a key template with only optional parts was accepted")
	}
}

// A profile's own rules replace the manifest's field by field.
func TestAProfilesRulesOverrideTheDefaults(t *testing.T) {
	base := Rules{Alias: DefaultAlias, Envs: []string{"prd", "stg"}, Enforce: EnforceWarn}
	got := base.Over(Rules{Alias: "{env}-{role}-{nn}", Start: 1})
	if got.Alias != "{env}-{role}-{nn}" || got.Start != 1 || got.Enforce != EnforceWarn || len(got.Envs) != 2 {
		t.Errorf("Over = %+v", got)
	}
}
