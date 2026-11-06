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
		!strings.Contains(out, "key_name       {profile}_{code}_{env}_{role}_{nn}-{algo}") {
		t.Fatalf("show:\n%s", out)
	}
}

// A profile with no code cannot have its servers named under a template that
// needs one, and says both ways out; with the code fallback it uses its name,
// and under a template without {code} it needs none.
func TestAProfileWithoutACode(t *testing.T) {
	p := editHome(t)
	run(t, "profile", "add", "work")
	out, err := runErr(t, "host", "add", "work", "--env", "prd", "--role", "web", "-H", "203.0.113.10", "-u", "ploi")
	if err == nil || !strings.Contains(err.Error()+out, "profile edit work --code") || !strings.Contains(err.Error()+out, "--code-fallback profile") {
		t.Fatalf("no code: %v\n%s", err, out)
	}
	run(t, "naming", "set", "--code-fallback", "profile")
	run(t, "host", "add", "work", "--env", "prd", "--role", "web", "-H", "203.0.113.10", "-u", "ploi")
	run(t, "profile", "add", "lab")
	run(t, "naming", "set", "--profile", "lab", "--alias", "{env?}-{role}-{nn}", "--start", "1")
	run(t, "host", "add", "lab", "--role", "gpu", "-H", "203.0.113.11", "-u", "ploi")
	run(t, "host", "add", "lab", "--env", "dev", "--role", "gpu", "-H", "203.0.113.12", "-u", "ploi")
	m, _ := manifest.Load(p.Manifest())
	got := []string{}
	for _, name := range []string{"work", "lab"} {
		for _, h := range m.Profiles[name].Hosts {
			got = append(got, h.Alias)
		}
	}
	if strings.Join(got, " ") != "work-prd-web-00 gpu-01 dev-gpu-01" {
		t.Fatalf("aliases = %v", got)
	}
}

// Setting a code, or any rule a key name depends on, never renames a key
// already minted: the hosts that would change keep their key names, written
// into the manifest; a host added afterwards takes the new name.
func TestANamingChangeNeverRenamesAKey(t *testing.T) {
	p := editHome(t)
	run(t, "profile", "add", "acme", "--category", "clients")
	run(t, "naming", "key-template", "{profile}_{code?}_{alias}-{algo}", "--yes")
	run(t, "host", "add", "acme", "web1", "-H", "203.0.113.10", "-u", "deploy")
	m, _ := manifest.Load(p.Manifest())
	before, _ := m.ResolvedKeyName("acme", m.Profiles["acme"].Hosts[0])
	if before != "acme_web1-ed25519" {
		t.Fatalf("before a code: %q", before)
	}
	if out := run(t, "profile", "edit", "acme", "--code", "acm"); !strings.Contains(out, "kept its key name: acme/web1") {
		t.Fatalf("setting the code kept nothing:\n%s", out)
	}
	run(t, "host", "add", "acme", "web2", "-H", "203.0.113.11", "-u", "deploy")
	m, _ = manifest.Load(p.Manifest())
	names := map[string]string{}
	for _, h := range m.Profiles["acme"].Hosts {
		names[h.Alias], _ = m.ResolvedKeyName("acme", h)
	}
	if names["web1"] != before || names["web2"] != "acme_acm_web2-ed25519" {
		t.Fatalf("key names = %v", names)
	}
}
