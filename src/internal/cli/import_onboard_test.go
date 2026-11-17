package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The documented onboarding, end to end: a hand-built ~/.ssh/config with
// plain hosts, a Match block and wildcard blocks, then `ssh import -y`, then
// `ssh reconcile -y`. Doctor used to fail straight after, because import left
// every block it copied where it was and the render defined each host twice
// (review R-BUG-09). Now the config is clean: in sync, nothing shadowed, no
// host defined twice, and the user's Match and wildcard blocks still there.
func TestOnboardingLeavesDoctorWithNothingToReportAboutTheConfig(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	if err := os.MkdirAll(p.SSHDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(p.SSHDir, "config")
	const messy = "Host *.lab\n    StrictHostKeyChecking accept-new\n\n" +
		"Host web\n    HostName web.example\n    User deploy\n\n" +
		"Match host *.corp exec \"test -n x\"\n    ForwardAgent no\n\n" +
		"Host db\n    HostName db.example\n    User ops\n    Port 2200\n\n" +
		"Host *\n    ServerAliveInterval 45\n"
	if err := os.WriteFile(cfg, []byte(messy), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runErr(t, "ssh", "import", cfg, "-y"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if out, err := runErr(t, "ssh", "reconcile", "-y", "--no-pin"); err != nil {
		t.Fatalf("reconcile: %v\n%s", err, out)
	}
	js, _ := runErr(t, "doctor", "--json")
	var rep struct {
		ConfigInSync     bool     `json:"config_in_sync"`
		ShadowedHosts    []string `json:"shadowed_hosts"`
		ConfigCollisions []string `json:"config_collisions"`
	}
	if err := json.Unmarshal([]byte(js[strings.Index(js, "{"):]), &rep); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, js)
	}
	if !rep.ConfigInSync || len(rep.ShadowedHosts) != 0 || len(rep.ConfigCollisions) != 0 {
		t.Errorf("doctor after onboarding: in sync=%v shadowed=%v collisions=%v",
			rep.ConfigInSync, rep.ShadowedHosts, rep.ConfigCollisions)
	}
	body, _ := os.ReadFile(cfg)
	for _, kept := range []string{"Host *.lab", "Match host *.corp", "ForwardAgent no", "ServerAliveInterval 45"} {
		if !strings.Contains(string(body), kept) {
			t.Errorf("onboarding lost %q:\n%s", kept, body)
		}
	}
	if out := run(t, "ssh", "config", "adopt", "--dry-run"); !strings.Contains(out, "no hand-written block") {
		t.Errorf("config adopt finds work after a clean onboarding:\n%s", out)
	}
}

// `ssh config adopt` brings a home onboarded the old way in line: the
// duplicate block goes, doctor's collision clears, and a second run is a no-op.
func TestConfigAdoptMigratesADuplicatedHost(t *testing.T) {
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "srv", "-H", "srv.example", "-u", "deploy")
	cfg := filepath.Join(p.SSHDir, "config")
	body, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// What an import before this change left: the user's block above the managed one.
	if err := os.WriteFile(cfg, append([]byte("Host srv\n    HostName srv.example\n    User deploy\n\n"), body...), 0o600); err != nil {
		t.Fatal(err)
	}
	if js, _ := runErr(t, "doctor", "--json"); !strings.Contains(js, "srv is also defined by") {
		t.Fatalf("doctor does not report the duplicate:\n%s", js)
	}
	dry := run(t, "ssh", "config", "adopt", "--dry-run")
	if !strings.Contains(dry, "would move") || !strings.Contains(dry, "-Host srv") {
		t.Errorf("dry run:\n%s", dry)
	}
	if out := run(t, "ssh", "config", "adopt", "-y"); !strings.Contains(out, "moved") {
		t.Errorf("adopt:\n%s", out)
	}
	if js, _ := runErr(t, "doctor", "--json"); strings.Contains(js, "srv is also defined by") {
		t.Errorf("doctor still reports the duplicate after adopt:\n%s", js)
	}
	after, _ := os.ReadFile(cfg)
	if out := run(t, "ssh", "config", "adopt", "-y"); !strings.Contains(out, "no hand-written block") {
		t.Errorf("a second adopt did something:\n%s", out)
	}
	if again, _ := os.ReadFile(cfg); string(again) != string(after) {
		t.Error("a second adopt changed the config")
	}
}
