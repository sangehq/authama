package web

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/hypervisor"
	"github.com/sangehq/authama/src/internal/services/dockerctx"
	"github.com/sangehq/authama/src/internal/services/vmsync"
)

func swapWebDocker(t *testing.T, available bool, cs []dockerctx.Context, check vmsync.Checker) *int {
	t.Helper()
	oldA, oldL, oldC := dockerAvailable, dockerList, dockerCheck
	calls := 0
	dockerAvailable = func() bool { return available }
	dockerList = func() ([]dockerctx.Context, error) { return cs, nil }
	dockerCheck = func(name, user string) dockerctx.Diagnosis {
		calls++
		if check != nil {
			return check(name, user)
		}
		return dockerctx.Diagnosis{State: dockerctx.DaemonOK}
	}
	t.Cleanup(func() { dockerAvailable, dockerList, dockerCheck = oldA, oldL, oldC })
	return &calls
}

// The VMs page lists the Docker contexts of VM hosts, each with what is wrong
// and the fix, offers the engine check, and leaves out contexts that are not
// authama's. Without ?check=docker it connects to nothing.
func TestVMsPageShowsDockerContexts(t *testing.T) {
	calls := swapWebDocker(t, true, []dockerctx.Context{
		{Name: "stale", Description: dockerctx.Marker, Endpoint: "ssh://stale"},
		{Name: "default", Endpoint: "unix:///var/run/docker.sock", Current: true},
	}, nil)
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	body := c.get("/vms").Body.String()
	for _, want := range []string{`id="docker-contexts-table"`, "<code>stale</code>", "no VM host stale is left",
		"authama ssh vm sync removes it", `/vms?check=docker#docker-contexts`, "Check the engines"} {
		if !strings.Contains(body, want) {
			t.Errorf("the VMs page has no %q", want)
		}
	}
	if strings.Contains(body, "<code>default</code>") {
		t.Error("a context that is not authama's is listed")
	}
	if strings.Contains(body, "engines checked just now") {
		t.Error("the page says it checked without being asked")
	}
	if *calls != 0 {
		t.Errorf("the page asked %d engine(s) without ?check=docker", *calls)
	}
	if body := c.get("/vms?check=docker").Body.String(); !strings.Contains(body, "engines checked just now") {
		t.Error("?check=docker does not say it checked")
	}
}

func TestVMsPageWithoutDocker(t *testing.T) {
	swapWebDocker(t, false, nil, nil)
	s, _, _ := newTestServer(t)
	body := signIn(t, s).get("/vms").Body.String()
	if !strings.Contains(body, "The docker CLI is not installed") || strings.Contains(body, "Check the engines") {
		t.Error("without docker the page does not say so, or still offers the check")
	}
}

// A hypervisor whose CLI hangs costs the page ListTimeout: the page says it
// timed out, names it, and still lists the rest.
func TestVMsPageSaysAHypervisorTimedOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	swapWebDocker(t, false, nil, nil)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "slowtool"), []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/bin:/usr/bin")
	t.Setenv("AUTHAMA_VM_HYPERVISORS", "slowbox")
	old := hypervisor.ListTimeout
	hypervisor.ListTimeout = 300 * time.Millisecond
	t.Cleanup(func() { hypervisor.ListTimeout = old })
	s, p, _ := newTestServer(t)
	// The page registers the home's command drivers from its authama.json.
	conf := `{"hypervisors": {"slowbox": {"list": ["slowtool", "list"], "address": ["slowtool", "ip", "{vm}"]}}}`
	if err := os.WriteFile(p.SettingsFile(), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hypervisor.Register(nil) })
	c := signIn(t, s)
	start := time.Now()
	body := c.get("/vms").Body.String()
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("/vms took %s with a hanging hypervisor", took)
	}
	for _, want := range []string{`id="vms-timed-out"`, "<strong>Timed out:</strong> slowbox did not answer within 300ms", ">timed out</span>"} {
		if !strings.Contains(body, want) {
			t.Errorf("/vms lacks %q", want)
		}
	}
}
