package setupxfer

import (
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// The document lists every key the setup uses, not only the ones a profile
// declares: each host's key too, once each.
func TestTheSetupDocumentListsEveryKeyItsHostsUse(t *testing.T) {
	src := home(t)
	m := manifest.Starter(false)
	own := "deploy"
	m.SetProfile("work", manifest.Profile{KeyScope: "per_service", Keys: []manifest.KeySpec{{Name: "main"}}, Hosts: []manifest.Host{
		{Alias: "web1", Hostname: "10.0.0.1"},
		{Alias: "web2", Hostname: "10.0.0.2", KeyName: &own},
		{Alias: "web3", Hostname: "10.0.0.3", KeyName: &own},
	}})
	shared := "personal"
	m.SetProfile("personal", manifest.Profile{KeyScope: "shared", KeyName: &shared, Hosts: []manifest.Host{
		{Alias: "box", Hostname: "10.0.0.4"},
		{Alias: "nas", Hostname: "10.0.0.5"},
	}})
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(src.Manifest()); err != nil {
		t.Fatal(err)
	}
	d, err := Build(src, "test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	web1, err := m.ResolvedKeyName("work", m.Profiles["work"].Hosts[0])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"personal/personal", "work/main", "work/" + web1, "work/deploy"}
	got := "," + strings.Join(d.Keys, ",") + ","
	for _, k := range want {
		if strings.Count(got, ","+k+",") != 1 {
			t.Errorf("keys %v: want %s exactly once", d.Keys, k)
		}
	}
	if len(d.Keys) != len(want) {
		t.Errorf("keys %v: want %d", d.Keys, len(want))
	}
}
