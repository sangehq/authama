package dockerctx

import "testing"

func TestParseListAndManaged(t *testing.T) {
	out := `{"Current":true,"Description":"Current DOCKER_HOST based configuration","DockerEndpoint":"unix:///var/run/docker.sock","Name":"default"}
{"Current":false,"Description":"` + Marker + `","DockerEndpoint":"ssh://devbox","Name":"devbox"}
{"Current":false,"Description":"","DockerEndpoint":"ssh://lab","Name":"lab"}
{"Current":false,"Description":"mine","DockerEndpoint":"ssh://elsewhere","Name":"other"}

`
	cs, err := ParseList([]byte(out))
	if err != nil || len(cs) != 4 {
		t.Fatalf("%+v %v", cs, err)
	}
	want := map[string]bool{"default": false, "devbox": true, "lab": true, "other": false}
	for _, c := range cs {
		if c.Managed() != want[c.Name] {
			t.Errorf("%s managed=%v", c.Name, c.Managed())
		}
	}
	if !cs[0].Current {
		t.Error("current not read")
	}
	if _, err := ParseList([]byte("{bad")); err == nil {
		t.Error("bad JSON accepted")
	}
}

func TestRemoveRefusesWhatAuthamaDoesNotManage(t *testing.T) {
	if _, err := Remove(Context{Name: "other", Endpoint: "ssh://elsewhere"}, t.TempDir()); err == nil {
		t.Error("removed a context authama did not make")
	}
	if _, err := Remove(Context{Name: "devbox", Endpoint: "ssh://devbox", Current: true}, t.TempDir()); err == nil {
		t.Error("removed the current context")
	}
}
