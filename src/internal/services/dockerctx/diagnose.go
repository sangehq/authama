package dockerctx

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Daemon states: what the guest user gets when they ask the Docker engine for
// its version. "denied" covers every way the command is there but the engine
// does not answer the user, so a context to it would fail on first use.
const (
	DaemonOK          = "ok"          // the engine answered
	DaemonAbsent      = "absent"      // no docker command
	DaemonDenied      = "denied"      // the socket refused this user
	DaemonDown        = "down"        // the engine is not running (or not where docker looks)
	DaemonUnreachable = "unreachable" // the SSH connection itself failed
	DaemonFailed      = "failed"      // anything else
)

// Diagnosis is one answer about a Docker engine: its state, the server
// version when it answered, and otherwise what failed and how to fix it.
type Diagnosis struct {
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// Healthy reports whether the engine answered.
func (d Diagnosis) Healthy() bool { return d.State == DaemonOK }

// Classify reads the error text of a failed `docker version` (run in the
// guest, or locally through a context) and names the cause and its fix. user
// is the guest user, "" when unknown. The fix is advice: authama never changes
// a guest's groups or services itself.
func Classify(output, user string) Diagnosis {
	out := strings.TrimSpace(output)
	low := strings.ToLower(out)
	d := Diagnosis{Detail: lastLines(out, 2)}
	who := user
	if who == "" {
		who = "<user>"
	}
	switch {
	// ssh's own failures first: the docker CLI wraps them in text that also
	// says "docker" and, for a refused key, "Permission denied".
	case strings.Contains(low, "exit status 255") || strings.Contains(low, "permission denied (publickey") ||
		strings.Contains(low, "host key verification failed") || strings.Contains(low, "connection timed out") ||
		strings.Contains(low, "could not resolve hostname") || strings.Contains(low, "connection refused") ||
		strings.Contains(low, "connection closed by"):
		d.State = DaemonUnreachable
		d.Fix = "check the host first: ssh <alias> true, then authama ssh vm sync"
	case strings.Contains(low, "command not found") || strings.Contains(low, "docker: not found") ||
		strings.Contains(low, "executable file not found"):
		d.State = DaemonAbsent
		d.Fix = "install Docker Engine in the guest (https://docs.docker.com/engine/install/)"
	case strings.Contains(low, "permission denied") && (strings.Contains(low, "docker daemon socket") || strings.Contains(low, ".sock")):
		d.State = DaemonDenied
		d.Fix = "in the guest: sudo usermod -aG docker " + who + ", then reconnect (a new login picks up the group)"
	case strings.Contains(low, "cannot connect to the docker daemon") || strings.Contains(low, "is the docker daemon running"):
		d.State = DaemonDown
		if strings.Contains(low, "/run/user/") {
			d.Fix = "start rootless Docker in the guest (systemctl --user enable --now docker), " +
				"and set DOCKER_HOST where a non-interactive ssh session reads it (the top of ~/.bashrc)"
		} else {
			d.Fix = "start the engine in the guest: sudo systemctl enable --now docker"
		}
	default:
		d.State = DaemonFailed
	}
	return d
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, " / ")
}

// checkTimeout bounds one Check: an unreachable guest must not hang doctor.
var checkTimeout = 20 * time.Second

// Check asks the engine behind context name for its version, through the
// same path every docker command takes (ssh, the ProxyCommand, the pin, the
// key and the daemon socket), so a pass means `docker --context name ...`
// works. It does not start anything: the caller checks only a running VM.
func Check(name, user string) Diagnosis {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "--context", name, "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return Diagnosis{State: DaemonUnreachable, Detail: "no answer in " + checkTimeout.String(),
			Fix: "check the host first: ssh " + name + " true"}
	}
	if err == nil {
		return Diagnosis{State: DaemonOK, Version: strings.TrimSpace(string(out))}
	}
	d := Classify(string(out), user)
	d.Fix = strings.ReplaceAll(d.Fix, "<alias>", name)
	return d
}
