package hypervisor

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseCommandListReadsJSONAndTabs(t *testing.T) {
	vms, err := ParseCommandList([]byte(`[{"name":"a","state":"running"},{"name":"b","state":"weird","running":true},{"name":"c"},{"state":"x"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(vms) != 3 || !vms[0].Running || !vms[1].Running || vms[2].Running || vms[2].State != "stopped" {
		t.Errorf("json: %+v", vms)
	}
	vms, err = ParseCommandList([]byte("one\trunning\r\ntwo\tsuspended\n\nthree\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vms) != 3 || !vms[0].Running || vms[1].Running || vms[1].State != "suspended" || vms[2].State != "unknown" {
		t.Errorf("tabs: %+v", vms)
	}
	if _, err := ParseCommandList([]byte("[not json")); err == nil {
		t.Error("bad JSON was accepted")
	}
}

func TestParseAddress(t *testing.T) {
	for in, want := range map[string]Endpoint{
		"10.0.0.5\n":        {Host: "10.0.0.5", Port: 22},
		"\n127.0.0.1:2222":  {Host: "127.0.0.1", Port: 2222},
		"[fe80::1]:2200":    {Host: "fe80::1", Port: 2200},
		"devbox.local\nzzz": {Host: "devbox.local", Port: 22},
	} {
		got, err := ParseAddress([]byte(in))
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "a b", "h:0", "h:99999"} {
		if _, err := ParseAddress([]byte(bad)); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestCommandSpecValidate(t *testing.T) {
	ok := CommandSpec{List: []string{"mytool", "list"}, Address: []string{"mytool", "ip", "{vm}"}}
	if probs := ok.Validate("mybox"); len(probs) != 0 {
		t.Errorf("a good spec: %v", probs)
	}
	for name, s := range map[string]CommandSpec{
		"parallels": ok,
		"Bad Name":  ok,
		"nolist":    {Address: ok.Address},
		"empty":     {List: []string{""}, Address: ok.Address},
		"ctrl":      {List: []string{"a\nb"}, Address: ok.Address},
	} {
		if len(s.Validate(name)) == 0 {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A command driver runs exactly the user's commands, with {vm} filled in.
func TestACommandDriverRunsTheConfiguredCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n  list) printf 'box1\\trunning\\nbox2\\tstopped\\n' ;;\n  ip) echo \"10.1.2.3:2201\" ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "mytool"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	Register(map[string]CommandSpec{
		"mybox":  {List: []string{"mytool", "list"}, Address: []string{"mytool", "ip", "{vm}"}},
		"broken": {List: []string{"mytool"}},
	})
	t.Cleanup(func() { Register(nil) })
	d, err := For("mybox")
	if err != nil {
		t.Fatal(err)
	}
	if !IsCustom(d) || !strings.HasPrefix(Describe(d), "custom") {
		t.Errorf("not a custom driver: %s", Describe(d))
	}
	if _, err := For("broken"); err == nil {
		t.Error("an invalid spec was registered")
	}
	vms, err := d.List()
	if err != nil || len(vms) != 2 || !vms[0].Running || vms[1].Running {
		t.Fatalf("list: %+v %v", vms, err)
	}
	ep, err := d.Address("box1")
	if err != nil || ep.Host != "10.1.2.3" || ep.Port != 2201 {
		t.Errorf("address: %+v %v", ep, err)
	}
	if CanExec(d) {
		t.Error("a driver with no exec command claims it can exec")
	}
}

func TestEnabledNames(t *testing.T) {
	get := func(v string) func(string) string {
		return func(k string) string {
			if k == "AUTHAMA_VM_HYPERVISORS" {
				return v
			}
			return ""
		}
	}
	if EnabledNames(get("")) != nil {
		t.Error("unset should enable every driver")
	}
	if got := strings.Join(EnabledNames(get("Parallels, vmpal\tlima")), ","); got != "parallels,vmpal,lima" {
		t.Errorf("got %s", got)
	}
}

// A CLI that does not answer is cut off at ListTimeout and reported as timed
// out, even when it leaves a child holding its output open.
func TestAHangingListTimesOut(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nsleep 30 &\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "slowtool"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/bin:/usr/bin")
	Register(map[string]CommandSpec{"slow": {List: []string{"slowtool", "list"}, Address: []string{"slowtool", "ip", "{vm}"}}})
	t.Cleanup(func() { Register(nil) })
	old := ListTimeout
	ListTimeout = 200 * time.Millisecond
	t.Cleanup(func() { ListTimeout = old })
	d, err := For("slow")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = d.List()
	if !errors.Is(err, ErrTimedOut) || !strings.Contains(err.Error(), "timed out after 200ms") {
		t.Fatalf("list: %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("a hanging list held the caller %s", took)
	}
}
