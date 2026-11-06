package manifest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/key"
)

// Profile is written by a hand-listed wire struct too; a field it omits is
// dropped on every save.
func TestEveryProfileFieldSurvivesSaving(t *testing.T) {
	var p Profile
	v := reflect.ValueOf(&p).Elem()
	var tags []string
	for i := 0; i < v.NumField(); i++ {
		tag := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		tags = append(tags, tag)
		f := v.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x")
		case reflect.Slice:
			f.Set(reflect.MakeSlice(f.Type(), 1, 1))
		case reflect.Ptr:
			f.Set(reflect.New(f.Type().Elem()))
		}
	}
	if len(tags) < 8 {
		t.Fatalf("inspected %d Profile fields", len(tags))
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	for _, tag := range tags {
		if _, ok := out[tag]; !ok {
			t.Errorf("profile.%s is set but MarshalJSON does not write it", tag)
		}
	}
}

func namingManifest(t *testing.T, n *Naming) *Manifest {
	t.Helper()
	m := Starter(false)
	m.Defaults.Naming = n
	m.SetProfile("simtabi", Profile{KeyScope: "per_service", Category: "orgs", Code: "simbi", Hosts: []Host{
		{Alias: "simbi-prd-web-00", Hostname: "203.0.113.10", User: "ploi"},
		{Alias: "oribi-web", Hostname: "203.0.113.11", User: "ploi"},
	}})
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	return m
}

// With no rules, or the default template, every key name is exactly the one
// earlier releases derived: nothing already minted is renamed.
func TestTheDefaultKeyNameIsUnchanged(t *testing.T) {
	for _, n := range []*Naming{nil, {KeyName: "{profile}_{alias}-{algo}"}, {Enforce: "warn"}} {
		m := namingManifest(t, n)
		for _, h := range m.Profiles["simtabi"].Hosts {
			got, err := m.ResolvedKeyName("simtabi", h)
			want, _ := key.DeriveKeyName("simtabi", h.Alias, "ed25519")
			if err != nil || got != want {
				t.Errorf("%+v: %s = %q, want %q", n, h.Alias, got, want)
			}
		}
	}
}

func TestAKeyNameTemplateNamesNewKeys(t *testing.T) {
	m := namingManifest(t, &Naming{KeyName: "{profile}_{code}_{env}_{role}_{nn}-{algo}"})
	hosts := m.Profiles["simtabi"].Hosts
	if got, _ := m.ResolvedKeyName("simtabi", hosts[0]); got != "simtabi_simbi_prd_web_00-ed25519" {
		t.Errorf("templated = %q", got)
	}
	// An alias the template cannot read its parts from keeps the old name.
	if got, _ := m.ResolvedKeyName("simtabi", hosts[1]); got != "simtabi_oribi-web-ed25519" {
		t.Errorf("fallback = %q", got)
	}
	if v := NamingViolation(m, "simtabi", "oribi-web"); !strings.Contains(v, "does not follow") {
		t.Errorf("violation = %q", v)
	}
	if v := NamingViolation(m, "simtabi", "simbi-prd-web-01"); v != "" {
		t.Errorf("a conforming alias reported %q", v)
	}
}

func TestNamingIsValidated(t *testing.T) {
	m := namingManifest(t, nil)
	m.Defaults.Naming = &Naming{KeyName: "{alias}-{algo}"}
	if m.Validate() == nil {
		t.Error("a key template without {profile}_ validated")
	}
	m.Defaults.Naming = nil
	m.SetProfile("other", Profile{KeyScope: "per_service", Code: "simbi"})
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "both have the code") {
		t.Errorf("a shared code: %v", err)
	}
	m.SetProfile("other", Profile{KeyScope: "per_service", Code: "Bad-Code"})
	if m.Validate() == nil {
		t.Error("an unusable code validated")
	}
}
