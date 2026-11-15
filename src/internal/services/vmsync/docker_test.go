package vmsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/dockerctx"
	"github.com/sangehq/authama/src/internal/services/knownhosts"
)

// The probe asks the engine, not only for the command: a guest user outside
// the docker group has the command and no working Docker. Restoring the old
// `command -v docker` probe fails this.
func TestTheProbeAsksTheEngineAsTheGuestUser(t *testing.T) {
	for _, want := range []string{"command -v docker", "docker version", "authama-docker=ok", "sh -c '"} {
		if !strings.Contains(dockerProbe, want) {
			t.Errorf("the probe has no %q: %s", want, dockerProbe)
		}
	}
}

func TestReadDockerProbe(t *testing.T) {
	for _, tc := range []struct {
		out    string
		docker bool
		state  string
	}{
		{"authama-docker=absent\n", false, dockerctx.DaemonAbsent},
		{"authama-docker=ok 27.3.1\n", true, dockerctx.DaemonOK},
		{"motd line\nauthama-docker=failed\npermission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock\n", true, dockerctx.DaemonDenied},
		{"authama-docker=failed\nCannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?\n", true, dockerctx.DaemonDown},
		{"authama-docker=failed\nweird\n", true, dockerctx.DaemonFailed},
		{"authama-docker=yes\n", true, ""},
		{"", false, ""},
	} {
		docker, d := readDockerProbe(tc.out)
		if docker != tc.docker || d.State != tc.state {
			t.Errorf("%q: docker=%v state=%q, want %v %q", tc.out, docker, d.State, tc.docker, tc.state)
		}
	}
	if _, d := readDockerProbe("authama-docker=ok 27.3.1\n"); d.Version != "27.3.1" {
		t.Errorf("version %q", d.Version)
	}
}

// A guest with docker whose engine refuses its user still gets its context
// (the context is right; the guest is not), and the host says how to fix the
// guest, naming the user.
func TestADeniedEngineGetsItsContextAndAFixNamingTheUser(t *testing.T) {
	s, _, dock := newTestSyncer(t)
	dock.contexts = nil
	s.Deps.Probe = func(string, string, int, []knownhosts.ScannedKey) ProbeResult {
		_, d := readDockerProbe("authama-docker=failed\npermission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock\n")
		return ProbeResult{Status: ProbeOK, Docker: true, Daemon: d}
	}
	plan, err := s.Plan()
	if err != nil {
		t.Fatal(err)
	}
	if !kinds(plan)[ActDockerCreate+" live"] {
		t.Fatalf("no context for a guest with docker: %v", plan.Lines())
	}
	var live Host
	for _, h := range plan.Hosts {
		if h.Alias == "live" {
			live = h
		}
	}
	if live.Docker == nil || live.Docker.State != dockerctx.DaemonDenied {
		t.Fatalf("the host does not carry the engine state: %+v", live)
	}
	if !strings.Contains(live.Note, "sudo usermod -aG docker me") {
		t.Errorf("the note does not name the fix for user me: %q", live.Note)
	}
	if !live.Running {
		t.Error("a running VM is not marked running")
	}
}

// A healthy engine adds nothing to the note.
func TestAHealthyEngineAddsNoNote(t *testing.T) {
	s, _, _ := newTestSyncer(t)
	s.Deps.Probe = func(string, string, int, []knownhosts.ScannedKey) ProbeResult {
		return ProbeResult{Status: ProbeOK, Docker: true, Daemon: dockerctx.Diagnosis{State: dockerctx.DaemonOK, Version: "27"}}
	}
	plan, _ := s.Plan()
	for _, h := range plan.Hosts {
		if strings.Contains(h.Note, "docker") {
			t.Errorf("%s: %q", h.Alias, h.Note)
		}
	}
}

// A repin or a removal first ends the host's shared connection, so no master
// keeps talking to the old guest.
func TestRepinAndRemovalCloseTheSharedConnection(t *testing.T) {
	s, _, _ := newTestSyncer(t)
	var closed []string
	s.Deps.CloseShared = func(_, alias, user string, port int) { closed = append(closed, alias+"/"+user) }
	_, _ = s.ApplyManifest(Plan{Actions: []Action{{Kind: ActRemoveHost, Profile: "dev", Alias: "old", user: "me"}}}, &strings.Builder{})
	_, _ = s.ApplyOutside(Plan{Actions: []Action{{Kind: ActRepin, Alias: "live", user: "me", token: knownhosts.HostToken("live", 0)}}}, &strings.Builder{})
	if strings.Join(closed, ",") != "old/me,live/me" {
		t.Errorf("closed %v", closed)
	}
}

func TestContextStatuses(t *testing.T) {
	plan := Plan{Hosts: []Host{
		{Profile: "dev", Alias: "live", State: "running", Running: true, User: "me"},
		{Profile: "dev", Alias: "sleepy", State: "suspended"},
		{Profile: "dev", Alias: "old", State: "gone"},
		{Profile: "dev", Alias: "moved", State: "running", Running: true},
	}}
	cs := []dockerctx.Context{
		{Name: "default", Endpoint: "unix:///var/run/docker.sock", Current: true},
		{Name: "orbstack", Endpoint: "unix:///Users/u/.orbstack/run/docker.sock"},
		{Name: "live", Description: dockerctx.Marker, Endpoint: "ssh://live"},
		{Name: "sleepy", Description: dockerctx.Marker, Endpoint: "ssh://sleepy"},
		{Name: "old", Description: dockerctx.Marker, Endpoint: "ssh://old"},
		{Name: "moved", Description: dockerctx.Marker, Endpoint: "ssh://elsewhere"},
		{Name: "stale", Description: dockerctx.Marker, Endpoint: "ssh://stale"},
	}
	var asked []string
	check := func(name, user string) dockerctx.Diagnosis {
		asked = append(asked, name+"/"+user)
		return dockerctx.Classify("permission denied ... unix:///var/run/docker.sock", user)
	}
	got := map[string]ContextStatus{}
	for _, c := range ContextStatuses(plan, cs, check) {
		got[c.Name] = c
	}
	if _, ok := got["default"]; ok {
		t.Error("a context that is not authama's and names no host is listed")
	}
	if _, ok := got["orbstack"]; ok {
		t.Error("OrbStack's own context is listed")
	}
	if strings.Join(asked, ",") != "live/me" {
		t.Errorf("checked %v; only the running VM with a correct context is asked", asked)
	}
	if c := got["live"]; c.Docker == nil || c.Problem == "" || !strings.Contains(c.Fix, "usermod -aG docker me") {
		t.Errorf("live: %+v", c)
	}
	if c := got["sleepy"]; c.Problem != "" || c.Docker != nil {
		t.Errorf("a suspended VM's context is judged or checked: %+v", c)
	}
	for name, want := range map[string]string{"old": "the VM is gone", "moved": "points at ssh://elsewhere", "stale": "no VM host stale"} {
		if !strings.Contains(got[name].Problem, want) {
			t.Errorf("%s: problem %q, want %q", name, got[name].Problem, want)
		}
	}
	if ContextStatuses(plan, cs, nil)[0].Docker != nil {
		t.Error("without a checker something was checked")
	}
}

// The probe is real shell: run it under /bin/sh, as the guest would, with a
// fake docker that answers, refuses, or is missing.
func TestTheProbeScriptRunsUnderSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the guest side is POSIX")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	for _, tc := range []struct {
		name, script, state string
	}{
		{"answers", "#!/bin/sh\necho 27.3.1\n", dockerctx.DaemonOK},
		{"refuses", "#!/bin/sh\necho 'permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock' >&2\nexit 1\n", dockerctx.DaemonDenied},
		{"missing", "", dockerctx.DaemonAbsent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			if tc.script != "" {
				if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(tc.script), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for _, tool := range []string{"sh", "tail", "printf"} {
				if p, err := exec.LookPath(tool); err == nil {
					_ = os.Symlink(p, filepath.Join(bin, tool))
				}
			}
			cmd := exec.Command(sh, "-c", dockerProbe)
			cmd.Env = []string{"PATH=" + bin}
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			if _, d := readDockerProbe(string(out)); d.State != tc.state {
				t.Errorf("state %q from %q, want %q", d.State, out, tc.state)
			}
		})
	}
}
