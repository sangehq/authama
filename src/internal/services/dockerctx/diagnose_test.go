package dockerctx

import (
	"strings"
	"testing"
)

// Each way `docker version` fails names its own cause and fix. ssh's own
// failures come first: the docker CLI wraps them in text that also says
// "docker" and, for a refused key, "Permission denied".
func TestClassifyNamesTheCauseAndTheFix(t *testing.T) {
	for _, tc := range []struct {
		out, state, fix string
	}{
		{"permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock: Get \"http://%2Fvar%2Frun%2Fdocker.sock/v1.24/version\": dial unix /var/run/docker.sock: connect: permission denied",
			DaemonDenied, "sudo usermod -aG docker dev"},
		{"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			DaemonDown, "sudo systemctl enable --now docker"},
		{"Cannot connect to the Docker daemon at unix:///run/user/1000/docker.sock. Is the docker daemon running?",
			DaemonDown, "systemctl --user enable --now docker"},
		{"error during connect: Get \"http://docker.example.com/v1.24/version\": command [ssh -- dev docker system dial-stdio] has exited with exit status 255, make sure the URL is valid, and Docker 18.09 or later is installed on the remote host: stderr=dev@dev: Permission denied (publickey).",
			DaemonUnreachable, "ssh <alias> true"},
		{"sh: 1: docker: not found", DaemonAbsent, "install Docker Engine"},
		{"something nobody has seen", DaemonFailed, ""},
	} {
		d := Classify(tc.out, "dev")
		if d.State != tc.state {
			t.Errorf("%q: state %s, want %s", tc.out, d.State, tc.state)
		}
		if tc.fix != "" && !strings.Contains(d.Fix, tc.fix) {
			t.Errorf("%q: fix %q, want it to contain %q", tc.out, d.Fix, tc.fix)
		}
		if d.Healthy() {
			t.Errorf("%q: a failure reads as healthy", tc.out)
		}
	}
	if d := Classify("permission denied ... docker.sock", ""); !strings.Contains(d.Fix, "<user>") {
		t.Errorf("an unknown user is not left as a placeholder: %q", d.Fix)
	}
}
