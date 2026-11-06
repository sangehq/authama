package cli

import (
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
)

func TestServersAreNamedByTheRules(t *testing.T) {
	p := editHome(t)
	run(t, "profile", "add", "simtabi", "--category", "orgs", "--code", "simbi")
	run(t, "naming", "set", "--envs", "prd,stg,dev", "--enforce", "error")

	run(t, "host", "add", "simtabi", "--env", "prd", "--role", "web", "-H", "203.0.113.10", "-u", "ploi")
	run(t, "host", "add", "simtabi", "--env", "prd", "--role", "web", "-H", "203.0.113.11", "-u", "ploi")
	run(t, "host", "add", "simtabi", "--env", "prd", "--role", "db", "-H", "203.0.113.20", "-u", "ploi")
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range m.Profiles["simtabi"].Hosts {
		got = append(got, h.Alias+"["+strings.Join(h.Tags, ",")+"]")
	}
	if strings.Join(got, " ") != "simbi-prd-web-00[web,prd] simbi-prd-web-01[web,prd] simbi-prd-db-00[db,prd]" {
		t.Fatalf("hosts = %v", got)
	}

	// error refuses an alias that breaks the rules; an env outside the list too.
	if out, err := runErr(t, "host", "add", "simtabi", "oribi-web", "-H", "203.0.113.30", "-u", "ploi"); err == nil {
		t.Fatalf("enforce=error accepted oribi-web:\n%s", out)
	}
	if _, err := runErr(t, "host", "add", "simtabi", "--env", "qa", "--role", "web", "-H", "203.0.113.31", "-u", "ploi"); err == nil {
		t.Fatal("an env outside the list was accepted")
	}
	// warn adds it and says why; check then lists it.
	run(t, "naming", "set", "--enforce", "warn")
	if out := run(t, "host", "add", "simtabi", "oribi-web", "-H", "203.0.113.30", "-u", "ploi"); !strings.Contains(out, "naming: oribi-web does not follow") {
		t.Fatalf("warn said nothing:\n%s", out)
	}
	if out := run(t, "naming", "check"); !strings.Contains(out, "simtabi: oribi-web does not follow") || strings.Contains(out, "simbi-prd-web-00") {
		t.Fatalf("check:\n%s", out)
	}
	if _, err := runErr(t, "naming", "check", "--strict"); err == nil {
		t.Fatal("check --strict passed with a stray alias")
	}

	// A new key template: every existing host keeps its key name, written into
	// the manifest; a host added afterwards follows the template.
	out := run(t, "naming", "key-template", "{profile}_{code}_{env}_{role}_{nn}-{algo}", "--yes")
	if !strings.Contains(out, "kept its key name: simtabi/simbi-prd-web-00") || strings.Contains(out, "oribi-web") {
		t.Fatalf("key-template:\n%s", out)
	}
	run(t, "host", "add", "simtabi", "--env", "stg", "--role", "web", "-H", "203.0.113.40", "-u", "ploi")
	m, _ = manifest.Load(p.Manifest())
	names := map[string]string{}
	for _, h := range m.Profiles["simtabi"].Hosts {
		kn, _ := m.ResolvedKeyName("simtabi", h)
		names[h.Alias] = kn
	}
	want := map[string]string{
		"simbi-prd-web-00": "simtabi_simbi-prd-web-00-ed25519",
		"oribi-web":        "simtabi_oribi-web-ed25519",
		"simbi-stg-web-00": "simtabi_simbi_stg_web_00-ed25519",
	}
	for alias, w := range want {
		if names[alias] != w {
			t.Errorf("%s: key %q, want %q", alias, names[alias], w)
		}
	}
	if out := run(t, "naming", "show"); !strings.Contains(out, "simtabi              code simbi") ||
		!strings.Contains(out, "key_name  {profile}_{code}_{env}_{role}_{nn}-{algo}") {
		t.Fatalf("show:\n%s", out)
	}
}

// A profile with no code cannot have its servers named, and says how to fix it.
func TestNamingAServerNeedsTheProfilesCode(t *testing.T) {
	editHome(t)
	run(t, "profile", "add", "work")
	out, err := runErr(t, "host", "add", "work", "--env", "prd", "--role", "web", "-H", "203.0.113.10", "-u", "ploi")
	if err == nil || !strings.Contains(err.Error()+out, "profile edit work --code") {
		t.Fatalf("no code: %v\n%s", err, out)
	}
}
