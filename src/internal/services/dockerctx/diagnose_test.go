package dockerctx

import (
	"context"
	"errors"
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

// Through an ssh context docker hides the guest's error behind a generic
// "Cannot connect ... at http://docker.example.com" (measured on a VMPal
// Ubuntu guest whose user was outside the docker group). Check then asks the
// guest, so the cause is "denied" with the usermod fix, not "down".
func TestCheckAsksTheGuestWhenTheContextHidesTheCause(t *testing.T) {
	oldV, oldA := dockerVersion, askGuest
	t.Cleanup(func() { dockerVersion, askGuest = oldV, oldA })
	dockerVersion = func(context.Context, string) ([]byte, error) {
		return []byte("Cannot connect to the Docker daemon at http://docker.example.com. Is the docker daemon running?"), errors.New("exit status 1")
	}
	asked := ""
	askGuest = func(name, user string) (Diagnosis, bool) {
		asked = name
		_, d := ReadGuestProbe("authama-docker=failed\npermission denied while trying to connect to the docker API at unix:///var/run/docker.sock\n")
		d.Fix = strings.ReplaceAll(d.Fix, "<user>", user)
		return d, true
	}
	d := Check("incus", "imanimanyara")
	if asked != "incus" || d.State != DaemonDenied || !strings.Contains(d.Fix, "usermod -aG docker imanimanyara") {
		t.Errorf("asked %q, got %+v", asked, d)
	}
	// No answer from the guest: keep what docker said.
	askGuest = func(string, string) (Diagnosis, bool) { return Diagnosis{}, false }
	if d := Check("incus", "u"); d.State != DaemonDown {
		t.Errorf("without the guest's answer: %+v", d)
	}
	// An error that does not hide the cause is not second-guessed.
	asked = ""
	dockerVersion = func(context.Context, string) ([]byte, error) {
		return []byte("permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock"), errors.New("exit status 1")
	}
	if d := Check("incus", "u"); d.State != DaemonDenied || asked != "" {
		t.Errorf("asked the guest when docker already said why: %+v %q", d, asked)
	}
}
