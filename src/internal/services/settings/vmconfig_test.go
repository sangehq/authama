package settings

import (
	"errors"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/hypervisor"
)

func TestTheVMSectionsParse(t *testing.T) {
	t.Cleanup(func() { hypervisor.Register(nil) })
	f, err := Parse("authama.json", []byte(`{
  "version": 1,
  "vm": {"hypervisors": "parallels,vmpal,mybox", "prune_gone": true},
  "hypervisors": {"mybox": {"description": "my box", "list": ["mybox", "ls"], "address": ["mybox", "ip", "{vm}"]}},
  "devboxes": {"devbox": {"profile": "devbox", "hypervisor": "parallels", "vm": "DEVBOX - Windows 11", "user": "me", "key": "devbox-key", "docker_context": false}}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Values["vm.hypervisors"] != "parallels,vmpal,mybox" {
		t.Errorf("values: %v", f.Values)
	}
	db, ok := f.VM.Devboxes["devbox"]
	if !ok || db.VM != "DEVBOX - Windows 11" || db.WantsDockerContext() {
		t.Errorf("devbox: %+v", db)
	}
	if _, ok := f.VM.Hypervisors["mybox"]; !ok {
		t.Error("the custom hypervisor was not read")
	}
}

func TestBadVMSectionsAreReported(t *testing.T) {
	t.Cleanup(func() { hypervisor.Register(nil) })
	for name, body := range map[string]string{
		"builtin name":  `{"hypervisors": {"parallels": {"list": ["x"], "address": ["x"]}}}`,
		"no address":    `{"hypervisors": {"mybox": {"list": ["x"]}}}`,
		"unknown field": `{"hypervisors": {"mybox": {"list": ["x"], "address": ["x"], "shell": "rm"}}}`,
		"unknown hv":    `{"devboxes": {"d": {"profile": "p", "hypervisor": "nope", "vm": "v", "user": "u"}}}`,
		"bad user":      `{"devboxes": {"d": {"profile": "p", "hypervisor": "parallels", "vm": "v", "user": "u; rm -rf"}}}`,
		"not an object": `{"devboxes": []}`,
	} {
		_, err := Parse("authama.json", []byte(body))
		var v *ValidationError
		if !errors.As(err, &v) || len(v.Problems) == 0 {
			t.Errorf("%s: accepted (%v)", name, err)
		}
	}
}

func TestValidUser(t *testing.T) {
	for _, u := range []string{"me", "dev.user", "_svc", "a-b"} {
		if !ValidUser(u) {
			t.Errorf("%q rejected", u)
		}
	}
	for _, u := range []string{"", "a b", "$(x)", "a;b", strings.Repeat("a", 40)} {
		if ValidUser(u) {
			t.Errorf("%q accepted", u)
		}
	}
}
