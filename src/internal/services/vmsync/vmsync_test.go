package vmsync

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/hypervisor"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/dockerctx"
	"github.com/sangehq/authama/src/internal/services/knownhosts"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/paths"
)

type fakeDriver struct {
	vms   []hypervisor.VM
	calls []string
}

func (f *fakeDriver) Name() string { return "lima" }
func (f *fakeDriver) CLI() string  { return "limactl" }
func (f *fakeDriver) List() ([]hypervisor.VM, error) {
	f.calls = append(f.calls, "list")
	return f.vms, nil
}
func (f *fakeDriver) Address(string) (hypervisor.Endpoint, error) {
	return hypervisor.Endpoint{Host: "127.0.0.1", Port: 2222}, nil
}
func (f *fakeDriver) Start(vm string) error { f.calls = append(f.calls, "start "+vm); return nil }
func (f *fakeDriver) Exec(vm string, _ ...string) ([]byte, error) {
	f.calls = append(f.calls, "exec "+vm)
	return nil, hypervisor.ErrNotSupported
}

type fakeDocker struct {
	contexts []dockerctx.Context
	calls    []string
}

func (f *fakeDocker) List() ([]dockerctx.Context, error) { return f.contexts, nil }
func (f *fakeDocker) Apply(a string) (string, error) {
	f.calls = append(f.calls, "apply "+a)
	return "created " + a, nil
}
func (f *fakeDocker) Remove(c dockerctx.Context, _ string) (string, error) {
	f.calls = append(f.calls, "remove "+c.Name)
	return "removed " + c.Name, nil
}

const vmManifest = `{
  "version": 1,
  "defaults": {"key_type": "ed25519", "rotate_after_days": 365},
  "profiles": {
    "dev": {"key_scope": "per_service", "hosts": [
      {"alias": "old", "hostname": "old", "user": "me", "vm": {"hypervisor": "lima", "name": "old"}},
      {"alias": "sleepy", "hostname": "sleepy", "user": "me", "vm": {"hypervisor": "lima", "name": "sleepy"}},
      {"alias": "live", "hostname": "live", "user": "me", "vm": {"hypervisor": "lima", "name": "live"}},
      {"alias": "other", "hostname": "other", "user": "me", "vm": {"hypervisor": "parallels", "name": "x"}},
      {"alias": "gh", "hostname": "github.com", "user": "git"}
    ]}
  }
}`

const testHostKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGVuZXJhdGVkIGZvciB0ZXN0cyBvbmx5IHgxMjM0NQ"

func newTestSyncer(t *testing.T) (*Syncer, *fakeDriver, *fakeDocker) {
	t.Helper()
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, "ssh"), ConfigDir: filepath.Join(root, "cfg")}
	for _, d := range []string{p.SSHDir, p.ConfigDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(p.Manifest(), []byte(vmManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	drv := &fakeDriver{vms: []hypervisor.VM{
		{Name: "sleepy", State: "suspended"},
		{Name: "live", State: "running", Running: true},
	}}
	dock := &fakeDocker{contexts: []dockerctx.Context{
		{Name: "old", Description: dockerctx.Marker, Endpoint: "ssh://old"},
		{Name: "mine", Endpoint: "unix:///var/run/docker.sock"},
	}}
	s := &Syncer{P: p, Cfg: settings.VMConfig{},
		Opt: Options{Probe: true, PruneGone: true, AutoPin: true, Docker: true},
		Deps: Deps{
			Driver:  func(string) (hypervisor.Driver, error) { return drv, nil },
			Enabled: func(name string) bool { return name == "lima" },
			Scan: func(_, host string, port int) ([]knownhosts.ScannedKey, error) {
				return []knownhosts.ScannedKey{{Host: host, Port: port, Keytype: "ssh-ed25519",
					Line: host + " " + testHostKey, Fingerprint: "SHA256:test"}}, nil
			},
			Probe: func(string, string, int, []knownhosts.ScannedKey) ProbeResult {
				return ProbeResult{Status: ProbeOK, Docker: true}
			},
			Docker: dock,
		}}
	return s, drv, dock
}

func kinds(p Plan) map[string]bool {
	out := map[string]bool{}
	for _, a := range p.Actions {
		out[a.Kind+" "+a.Alias] = true
	}
	return out
}

func TestPlanCoversGoneSuspendedRunningAndDisabled(t *testing.T) {
	s, drv, _ := newTestSyncer(t)
	plan, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	k := kinds(plan)
	for _, want := range []string{ActRemoveHost + " old", ActDockerRemove + " old", ActPin + " live"} {
		if !k[want] {
			t.Errorf("plan lacks %q: %v", want, plan.Lines())
		}
	}
	for a := range k {
		if strings.HasSuffix(a, " sleepy") || strings.HasSuffix(a, " other") || strings.HasSuffix(a, " gh") || strings.HasSuffix(a, " mine") {
			t.Errorf("plan touches something it must leave alone: %s", a)
		}
	}
	states := map[string]string{}
	for _, h := range plan.Hosts {
		states[h.Alias] = h.State
	}
	if states["old"] != "gone" || states["sleepy"] != "suspended" || states["live"] != "running" || states["other"] != "disabled" {
		t.Errorf("states: %v", states)
	}
	for _, c := range drv.calls {
		if strings.HasPrefix(c, "start") || strings.HasPrefix(c, "exec") {
			t.Errorf("planning acted on a VM: %v", drv.calls)
		}
	}
}

func TestNoProbePlanConnectsToNoGuest(t *testing.T) {
	s, _, _ := newTestSyncer(t)
	s.Opt.Probe = false
	s.Deps.Scan = func(string, string, int) ([]knownhosts.ScannedKey, error) {
		t.Error("a no-probe plan scanned a guest")
		return nil, nil
	}
	plan, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if !kinds(plan)[ActRemoveHost+" old"] {
		t.Errorf("a no-probe plan should still see the gone VM: %v", plan.Lines())
	}
}

func TestAnEmptyListingNeverPrunes(t *testing.T) {
	s, drv, _ := newTestSyncer(t)
	drv.vms = nil
	plan, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if kinds(plan)[ActRemoveHost+" old"] {
		t.Error("a hypervisor that lists nothing made a host look gone")
	}
}

func TestPruneGoneOffAndDeclaredDevboxesKeepTheHost(t *testing.T) {
	s, _, _ := newTestSyncer(t)
	s.Opt.PruneGone = false
	if plan, _ := s.Plan(); kinds(plan)[ActRemoveHost+" old"] {
		t.Error("prune_gone off still removed the host")
	}
	s.Opt.PruneGone = true
	s.Cfg.Devboxes = map[string]settings.Devbox{"old": {Profile: "dev", Hypervisor: "lima", VM: "old", User: "me"}}
	if plan, _ := s.Plan(); kinds(plan)[ActRemoveHost+" old"] {
		t.Error("a declared devbox was pruned")
	}
}

func TestApplyBacksUpAndIsIdempotent(t *testing.T) {
	s, _, dock := newTestSyncer(t)
	plan, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	backup, err := s.BackupManifest()
	if err != nil || backup == "" {
		t.Fatalf("backup: %q %v", backup, err)
	}
	if b, err := os.ReadFile(backup); err != nil || !strings.Contains(string(b), `"old"`) {
		t.Fatalf("the backup does not hold the manifest: %v", err)
	}
	if _, err := s.ApplyManifest(plan, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyOutside(plan, io.Discard); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(s.P.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range m.Profiles["dev"].Hosts {
		if h.Alias == "old" {
			t.Error("the gone host is still in the manifest")
		}
	}
	got := strings.Join(dock.calls, ",")
	if !strings.Contains(got, "remove old") || strings.Contains(got, "mine") {
		t.Errorf("docker calls: %v", dock.calls)
	}
	// Once more: the gone host and its context are gone and the pin is in
	// place, so there is nothing left to do.
	dock.contexts = []dockerctx.Context{{Name: "live", Description: dockerctx.Marker, Endpoint: "ssh://live"}}
	again, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() {
		t.Errorf("a second sync still has work: %v", again.Lines())
	}
}
