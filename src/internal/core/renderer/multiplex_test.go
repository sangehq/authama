package renderer

import (
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/envnames"
)

func pinMultiplex(t *testing.T, on bool) {
	t.Helper()
	old := Multiplex
	Multiplex = func() bool { return on }
	t.Cleanup(func() { Multiplex = old })
}

func renderedAll(t *testing.T, json string) string {
	t.Helper()
	files, err := RenderAll(mustManifest(t, json), false)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, c := range files {
		b.WriteString(c)
	}
	return b.String()
}

// A VM host shares one connection, so a run of docker commands asks the
// hypervisor for the address once; any other host does not.
func TestAVMHostSharesItsConnection(t *testing.T) {
	pinSelf(t, "/opt/homebrew/bin/authama")
	pinMultiplex(t, true)
	out := renderedAll(t, gitVMJSON)
	want := "    ProxyCommand \"/opt/homebrew/bin/authama\" ssh vm proxy devbox\n" +
		"    ControlMaster auto\n    ControlPath ~/.ssh/" + ControlDir + "/%C\n    ControlPersist 10m\n"
	if !strings.Contains(out, want) {
		t.Errorf("the devbox block does not share its connection:\n%s", out)
	}
	if n := strings.Count(out, "ControlMaster"); n != 1 {
		t.Errorf("%d ControlMaster lines; only the VM host gets one:\n%s", n, out)
	}
}

func TestMultiplexOffRendersNoControlLines(t *testing.T) {
	pinMultiplex(t, false)
	if out := renderedAll(t, gitVMJSON); strings.Contains(out, "Control") {
		t.Errorf("vm.multiplex off still renders Control lines:\n%s", out)
	}
}

// A value the host sets itself in raw_options wins, and is written once.
func TestARawControlOptionWins(t *testing.T) {
	pinMultiplex(t, true)
	json := strings.Replace(gitVMJSON, `"vm": {"hypervisor"`, `"raw_options": {"ControlPersist": "1h"}, "vm": {"hypervisor"`, 1)
	out := renderedAll(t, json)
	if strings.Contains(out, "ControlPersist 10m") || strings.Count(out, "ControlPersist") != 1 || !strings.Contains(out, "ControlPersist 1h") {
		t.Errorf("raw ControlPersist did not win cleanly:\n%s", out)
	}
	if !strings.Contains(out, "ControlMaster auto") {
		t.Errorf("the other Control lines went too:\n%s", out)
	}
}

func TestMultiplexSetting(t *testing.T) {
	for v, want := range map[string]bool{"": true, "1": true, "true": true, "on": true, "0": false, "false": false, "off": false, "no": false} {
		t.Setenv(envnames.VMMultiplex, v)
		got := defaultMultiplex()
		if runtime.GOOS == "windows" {
			want = false
		}
		if got != want {
			t.Errorf("%s=%q: %v, want %v", envnames.VMMultiplex, v, got, want)
		}
	}
}
