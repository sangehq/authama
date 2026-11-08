package doctor

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/dockerctx"
	"github.com/simtabi/authama/src/internal/services/preflight"
	"github.com/simtabi/authama/src/internal/services/vmsync"
)

func swapDocker(t *testing.T, cs []dockerctx.Context, err error, check vmsync.Checker) {
	t.Helper()
	oldL, oldC := DockerList, DockerCheck
	DockerList = func() ([]dockerctx.Context, error) { return cs, err }
	DockerCheck = check
	t.Cleanup(func() { DockerList, DockerCheck = oldL, oldC })
}

// doctor asks the engine behind each running VM's context, reports one that
// does not answer as a warning with its fix (failing --strict), and shows a
// healthy one with its version.
func TestDoctorChecksTheDockerContextsOfVMHosts(t *testing.T) {
	swapDocker(t, []dockerctx.Context{
		{Name: "devbox", Description: dockerctx.Marker, Endpoint: "ssh://devbox"},
		{Name: "build", Description: dockerctx.Marker, Endpoint: "ssh://build"},
		{Name: "default", Endpoint: "unix:///var/run/docker.sock", Current: true},
	}, nil, func(name, user string) dockerctx.Diagnosis {
		if name == "devbox" {
			return dockerctx.Classify("permission denied while trying to connect to the Docker daemon socket", user)
		}
		return dockerctx.Diagnosis{State: dockerctx.DaemonOK, Version: "27.3.1"}
	})
	plan := vmsync.Plan{Hosts: []vmsync.Host{
		{Profile: "dev", Alias: "devbox", State: "running", Running: true, User: "dev"},
		{Profile: "dev", Alias: "build", State: "running", Running: true, User: "ci"},
	}}
	r := Report{ConfigInSync: true, Preflight: preflight.Report{RuntimeOK: true}, DockerContexts: dockerStatus(plan)}
	out := r.Format()
	for _, want := range []string{"Docker contexts of VM hosts:", "devbox -> ssh://devbox  WARNING", "sudo usermod -aG docker dev",
		"build -> ssh://build  ok, engine 27.3.1"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "default ->") {
		t.Error("a context that is not authama's is reported")
	}
	if !r.OK() {
		t.Error("an engine problem failed the verdict without --strict")
	}
	r.Strict = true
	if r.OK() {
		t.Error("--strict passed a context whose engine refuses its user")
	}
	b, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		DockerContexts []vmsync.ContextStatus `json:"docker_contexts"`
	}
	if err := json.Unmarshal(b, &doc); err != nil || len(doc.DockerContexts) != 2 {
		t.Errorf("JSON docker_contexts: %v %s", err, b)
	}
}

func TestDoctorReportsADockerListFailureAndNothingWithoutContexts(t *testing.T) {
	swapDocker(t, nil, errors.New("docker context ls: boom"), nil)
	if cs := dockerStatus(vmsync.Plan{}); len(cs) != 1 || !strings.Contains(cs[0].Problem, "boom") {
		t.Errorf("%+v", cs)
	}
	swapDocker(t, nil, nil, nil)
	r := Report{ConfigInSync: true, Preflight: preflight.Report{RuntimeOK: true}, DockerContexts: dockerStatus(vmsync.Plan{})}
	if strings.Contains(r.Format(), "Docker contexts") {
		t.Error("a machine with no VM contexts gets a Docker section")
	}
	b, _ := r.JSON()
	if !strings.Contains(string(b), `"docker_contexts": []`) {
		t.Errorf("docker_contexts is not an empty list: %s", b)
	}
}
