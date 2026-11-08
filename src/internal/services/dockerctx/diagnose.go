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

// GuestProbe asks a guest, as the guest user over a non-interactive ssh
// session (the session `docker system dial-stdio` runs in), whether it has
// docker and whether the engine answers. Wrapped in sh -c so the guest's login
// shell (bash, zsh, fish) does not matter. ReadGuestProbe reads its output.
const GuestProbe = `sh -c 'if ! command -v docker >/dev/null 2>&1; then echo authama-docker=absent; ` +
	`elif v=$(docker version --format "{{.Server.Version}}" 2>&1); then echo "authama-docker=ok $v"; ` +
	`else echo authama-docker=failed; printf "%s\n" "$v" | tail -n 3; fi'`

// ReadGuestProbe reads GuestProbe's output: the marker line, and after a
// failure the last lines of docker's error. It reports whether the guest has
// docker, and the engine's state ("" when no marker came back).
func ReadGuestProbe(stdout string) (bool, Diagnosis) {
	lines := strings.Split(stdout, "\n")
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "authama-docker=") {
			continue
		}
		state, rest, _ := strings.Cut(strings.TrimPrefix(l, "authama-docker="), " ")
		switch state {
		case "absent":
			return false, Diagnosis{State: DaemonAbsent}
		case "ok":
			return true, Diagnosis{State: DaemonOK, Version: strings.TrimSpace(rest)}
		case "yes": // an older probe: the command exists, the engine was not asked
			return true, Diagnosis{}
		default:
			d := Classify(strings.Join(lines[i+1:], "\n"), "")
			if d.State == DaemonAbsent || d.State == DaemonUnreachable {
				d.State = DaemonFailed // the command ran, over a working connection
			}
			return true, d
		}
	}
	return false, Diagnosis{}
}

// connhelperHost is the placeholder the docker CLI names for an ssh://
// context. An error naming it says only that the guest's `docker system
// dial-stdio` failed; the guest's own reason is not passed on.
const connhelperHost = "docker.example.com"

// checkTimeout bounds one Check: an unreachable guest must not hang doctor.
var checkTimeout = 20 * time.Second

// Check asks the engine behind context name for its version, through the
// same path every docker command takes (ssh, the ProxyCommand, the pin, the
// key and the daemon socket), so a pass means `docker --context name ...`
// works. It does not start anything: the caller checks only a running VM.
func Check(name, user string) Diagnosis {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	out, err := dockerVersion(ctx, name)
	if ctx.Err() == context.DeadlineExceeded {
		return Diagnosis{State: DaemonUnreachable, Detail: "no answer in " + checkTimeout.String(),
			Fix: "check the host first: ssh " + name + " true"}
	}
	if err == nil {
		return Diagnosis{State: DaemonOK, Version: strings.TrimSpace(string(out))}
	}
	if strings.Contains(string(out), connhelperHost) {
		// Through an ssh context the guest's error is hidden behind a generic
		// "Cannot connect ... at http://docker.example.com", which would read as
		// a stopped engine when the socket refused the user. Ask the guest.
		if d, ok := askGuest(name, user); ok {
			return d
		}
	}
	d := Classify(string(out), user)
	d.Fix = strings.ReplaceAll(d.Fix, "<alias>", name)
	return d
}

// dockerVersion runs `docker --context name version`; tests swap it.
var dockerVersion = func(ctx context.Context, name string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", "--context", name, "version", "--format", "{{.Server.Version}}").CombinedOutput()
}

// askGuest runs GuestProbe on host name over ssh (the user's config, the
// pinned key, BatchMode) and reads the guest's own answer. ok is false when no
// answer came back, so the caller keeps what docker said.
var askGuest = func(name, user string) (Diagnosis, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		"--", name, GuestProbe).Output()
	if err != nil && len(out) == 0 {
		return Diagnosis{}, false
	}
	has, d := ReadGuestProbe(string(out))
	if d.State == "" {
		return Diagnosis{}, false
	}
	if !has && d.State == DaemonAbsent {
		d.Fix = "install Docker Engine in the guest (https://docs.docker.com/engine/install/)"
	}
	if d.State == DaemonOK {
		// The guest's engine answers its user, so the context path itself failed.
		return Diagnosis{State: DaemonFailed, Version: d.Version,
			Detail: "the guest's engine answers over ssh, but not through the context",
			Fix:    "recreate the context: authama ssh vm docker-context " + name}, true
	}
	if user != "" {
		d.Fix = strings.ReplaceAll(d.Fix, "<user>", user)
	}
	return d, true
}
