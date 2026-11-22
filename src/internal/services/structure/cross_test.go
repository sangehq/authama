package structure

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
)

func TestCrossProfileLinesParse(t *testing.T) {
	moves, err := ParsePlan("host work/gh profile home\nkey work/deploy profile home name deploy2\nkey work/x profile home")
	if err != nil {
		t.Fatal(err)
	}
	want := []Move{
		{Kind: MoveHost, From: "work", Name: "gh", Profile: "home"},
		{Kind: MoveDeclared, From: "work", Key: "deploy", Name: "deploy2", Profile: "home"},
		{Kind: MoveDeclared, From: "work", Key: "x", Name: "x", Profile: "home"},
	}
	for i, mv := range moves {
		if mv != want[i] {
			t.Errorf("move %d = %+v, want %+v", i, mv, want[i])
		}
	}
	if moves[0].Line() != "host work/gh profile home" || moves[1].Line() != "key work/deploy profile home name deploy2" {
		t.Errorf("lines: %q %q", moves[0].Line(), moves[1].Line())
	}
	for _, bad := range []string{"host gh profile home", "host work/../x profile home", "key work/a profile ../b", "host work/gh profile -x"} {
		if _, err := ParsePlan(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	// A path source is still a stray key's move.
	if mv, err := ParsePlan("key ~/.ssh/id_x profile home"); err != nil || mv[0].Kind != MoveKey {
		t.Errorf("a path source: %+v %v", mv, err)
	}
}

// A host moved to another profile takes its own key along - file pair,
// inventory record and deployments - and every hand-written line naming it
// is repointed; a key still used by a host staying behind does not move.
func TestAHostMovesToAnotherProfileWithItsKey(t *testing.T) {
	p := home(t, `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","hosts":[{"alias":"gh","hostname":"github.com","user":"git"},
	                                           {"alias":"box","hostname":"box.example","user":"me"}]},
	  "home":{"key_scope":"per_service","hosts":[]}}}`)
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	key, err := m.ResolvedKeyName("work", m.Profiles["work"].Hosts[0])
	if err != nil {
		t.Fatal(err)
	}
	ref := manifest.KeyRef{Profile: "work", KeyName: key}
	src := m.KeyPath(p.SSHDir, ref)
	pair(t, src, pubWork)
	inv := inventory.New()
	inv.Record("SHA256:fixture", inventory.KeyRecord{Profile: "work", Path: m.IdentityFile("work", key), Type: "ed25519",
		Deployments: []inventory.Deployment{{Target: "gh", Method: "web-panel", Verified: true}}})
	if err := inv.Save(p.Inventory()); err != nil {
		t.Fatal(err)
	}
	write(t, p.SSHDir+"/config", "Host extra\n  IdentityFile "+src+"\n", 0o600)

	moves, err := ParsePlan("host work/gh profile home")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res, err := Apply(p, moves, &out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	m2, _ := manifest.Load(p.Manifest())
	if len(m2.Profiles["work"].Hosts) != 1 || len(m2.Profiles["home"].Hosts) != 1 || m2.Profiles["home"].Hosts[0].Alias != "gh" {
		t.Fatalf("hosts: work %+v home %+v", m2.Profiles["work"].Hosts, m2.Profiles["home"].Hosts)
	}
	dest := m2.KeyPath(p.SSHDir, manifest.KeyRef{Profile: "home", KeyName: key})
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("the key did not move to %s: %v\n%s", dest, err, out.String())
	}
	if _, err := os.Lstat(src); !os.IsNotExist(err) {
		t.Error("the key is still in the old profile")
	}
	if k, _ := m2.ResolvedKeyName("home", m2.Profiles["home"].Hosts[0]); k != key {
		t.Errorf("the moved host resolves %q, want its own key %q", k, key)
	}
	inv2, _ := inventory.Load(p.Inventory())
	rec := inv2.Keys["SHA256:fixture"]
	if rec.Profile != "home" || rec.Path != m2.IdentityFile("home", key) || len(rec.Deployments) != 1 {
		t.Errorf("record: %+v", rec)
	}
	if b, _ := os.ReadFile(p.SSHDir + "/config"); strings.Contains(string(b), "profiles/work/") {
		t.Errorf("config not repointed:\n%s", b)
	}
	if res.Backup == "" {
		t.Error("no backup of the moved pair")
	}
}

// A key a host still uses cannot move without that host; with it, both move,
// and the host keeps the key under its new name.
func TestADeclaredKeyMovesOnlyWithItsHosts(t *testing.T) {
	p := home(t, `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","keys":[{"name":"deploy"}],"hosts":[{"alias":"ci","hostname":"ci.example","user":"git","key_name":"deploy"}]},
	  "home":{"key_scope":"per_service","hosts":[]}}}`)
	m, _ := manifest.Load(p.Manifest())
	src := m.KeyPath(p.SSHDir, manifest.KeyRef{Profile: "work", KeyName: "deploy"})
	pair(t, src, pubWork)

	moves, _ := ParsePlan("key work/deploy profile home name deploy-home")
	var out bytes.Buffer
	if _, err := Apply(p, moves, &out); err == nil || !strings.Contains(err.Error(), "host ci still uses it") {
		t.Fatalf("moved a key its host still uses: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("a refused move touched the key")
	}

	moves, _ = ParsePlan("key work/deploy profile home name deploy-home\nhost work/ci profile home")
	if _, err := Apply(p, moves, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	m2, _ := manifest.Load(p.Manifest())
	h := m2.Profiles["home"].Hosts[0]
	if h.Alias != "ci" || h.KeyName == nil || *h.KeyName != "deploy-home" {
		t.Errorf("host: %+v", h)
	}
	if _, ok := m2.KeySpecFor(manifest.KeyRef{Profile: "home", KeyName: "deploy-home"}); !ok {
		t.Error("the declaration did not follow the key")
	}
	if _, ok := m2.KeySpecFor(manifest.KeyRef{Profile: "work", KeyName: "deploy"}); ok {
		t.Error("the old profile still declares it")
	}
	if _, err := os.Stat(m2.KeyPath(p.SSHDir, manifest.KeyRef{Profile: "home", KeyName: "deploy-home"})); err != nil {
		t.Error("the pair did not move")
	}
}
