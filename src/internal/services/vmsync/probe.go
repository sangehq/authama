package vmsync

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/services/knownhosts"
)

// Probe outcomes.
const (
	ProbeOK          = "ok"          // the guest accepted the key
	ProbeKeyRefused  = "key-refused" // the guest answered and refused every key
	ProbeUnreachable = "unreachable" // anything else: timeout, host key, proxy
)

// ProbeResult is what one BatchMode ssh to the guest found.
type ProbeResult struct {
	Status string
	Docker bool   // the guest has a docker command
	Detail string // why, when not ok
}

const dockerProbe = "command -v docker >/dev/null 2>&1 && echo authama-docker=yes || echo authama-docker=no"

// probeTimeout bounds one probe.
var probeTimeout = 30 * time.Second

// sshProbe connects once through the user's config, non-interactively, and
// trusting exactly the host keys just scanned (in a throwaway known_hosts) so
// the result is about the key, not the pin.
func sshProbe(sshDir, alias string, port int, scanned []knownhosts.ScannedKey) ProbeResult {
	if _, err := exec.LookPath("ssh"); err != nil {
		return ProbeResult{Status: ProbeUnreachable, Detail: "ssh not found on PATH"}
	}
	tmp, err := os.MkdirTemp("", "authama-vmsync-")
	if err != nil {
		return ProbeResult{Status: ProbeUnreachable, Detail: err.Error()}
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	kh := filepath.Join(tmp, "known_hosts")
	token := knownhosts.HostToken(alias, port)
	var lines []string
	for _, k := range scanned {
		if f := strings.Fields(k.Line); len(f) >= 3 {
			lines = append(lines, token+" "+f[1]+" "+f[2])
		}
	}
	if err := os.WriteFile(kh, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return ProbeResult{Status: ProbeUnreachable, Detail: err.Error()}
	}
	args := []string{
		"-F", filepath.Join(sshDir, "config"),
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=10",
		"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + kh,
		"-o", "GlobalKnownHostsFile=/dev/null", "-o", "LogLevel=ERROR",
		"--", alias, dockerProbe,
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = nil
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return classify(err, stdout.String(), stderr.String())
}

// classify reads an ssh run: exit 255 is ssh's own failure, anything else
// came from the remote command, so the key was accepted.
func classify(err error, stdout, stderr string) ProbeResult {
	var ee *exec.ExitError
	if err != nil && (!errors.As(err, &ee) || ee.ExitCode() == 255) {
		msg := strings.TrimSpace(stderr)
		if strings.Contains(msg, "Permission denied") {
			return ProbeResult{Status: ProbeKeyRefused, Detail: lastLine(msg)}
		}
		if msg == "" {
			msg = err.Error()
		}
		return ProbeResult{Status: ProbeUnreachable, Detail: lastLine(msg)}
	}
	return ProbeResult{Status: ProbeOK, Docker: strings.Contains(stdout, "authama-docker=yes")}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
