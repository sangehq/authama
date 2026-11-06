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
		"{code}-{env}-{role}":       "no {nn}",
		"{code}{env}-{nn}":          "touch",
		"{code}-{zone}-{nn}":        "not a part",
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
