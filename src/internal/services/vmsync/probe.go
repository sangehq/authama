package vmsync

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/renderer"
	"github.com/simtabi/authama/src/internal/services/dockerctx"
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
	// Daemon is whether the guest user reaches the Docker engine: a docker
	// command alone is not enough (the user may not be in the docker group,
	// or the engine may be stopped). Empty State when the probe did not run.
	Daemon dockerctx.Diagnosis
}

// dockerProbe asks the guest, as the guest user over a non-interactive ssh
// session, whether it has docker and whether the engine answers
// (dockerctx.GuestProbe, which doctor uses too).
const dockerProbe = dockerctx.GuestProbe

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
		// A fresh connection: a shared master (vm.multiplex) would skip the
		// host-key check this probe exists to make.
		"-o", "ControlMaster=no", "-o", "ControlPath=none",
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
	pr := ProbeResult{Status: ProbeOK}
	pr.Docker, pr.Daemon = readDockerProbe(stdout)
	return pr
}

// readDockerProbe reads dockerProbe's output (dockerctx.ReadGuestProbe).
func readDockerProbe(stdout string) (bool, dockerctx.Diagnosis) {
	return dockerctx.ReadGuestProbe(stdout)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// closeShared asks a VM host's shared connection to exit. It names the socket
// the way the rendered config does (ControlPath <ssh dir>/authama-cm/%C, with
// the alias as the host, as a VM host's HostName is), so it works after the
// host has left the config too. No master, or no ssh, is not an error.
func closeShared(sshDir, alias, user string, port int) {
	dir := filepath.Join(sshDir, renderer.ControlDir)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if port == 0 {
		port = 22
	}
	args := []string{"-F", os.DevNull, "-o", "ControlPath=" + filepath.Join(dir, "%C"), "-p", strconv.Itoa(port)}
	if user != "" {
		args = append(args, "-l", user)
	}
	args = append(args, "-O", "exit", alias)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "ssh", args...).Run()
}
