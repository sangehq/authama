// Package svchealth runs read-only checks for reachability services
// (cloudflared, WARP, Tailscale, and the other built-in kinds). It never
// starts a tunnel, changes WARP, or opens a browser.
package svchealth

import (
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// Report is one service's health.
type Report struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Installed bool   `json:"installed"`
	Binary    string `json:"binary,omitempty"`
	Version   string `json:"version,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
}

// LookPath and Run are test seams. Run must not be used to change system state.
var LookPath = exec.LookPath
var Run = func(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.WaitDelay = 8 * time.Second
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Check reports health for name (a built-in kind or a catalog ref).
func Check(m *manifest.Manifest, name string) Report {
	kind := manifest.ServiceKind(m, name)
	binary := binaryFor(m, name, kind)
	r := Report{Name: name, Kind: kind, Status: "unknown"}
	if binary == "" {
		r.Status = "unsupported"
		r.Detail = "no local health check for this kind"
		return r
	}
	path, err := LookPath(binary)
	if err != nil {
		r.Status = "not installed"
		r.Detail = binary + " is not on PATH"
		return r
	}
	r.Installed, r.Binary = true, path
	switch kind {
	case manifest.ServiceCloudflareAccess:
		out, _ := Run(path, "--version")
		r.Version = firstLine(out)
		r.Status, r.Detail = "installed", "read-only version check; does not test an Access login"
	case manifest.ServiceCloudflareWARP:
		out, err := Run(path, "status")
		r.Detail = firstLine(out)
		if err != nil {
			r.Status = "error"
			return r
		}
		low := strings.ToLower(out)
		if strings.Contains(low, "connected") {
			r.Status = "connected"
		} else if strings.Contains(low, "disconnected") || strings.Contains(low, "not connected") {
			r.Status = "disconnected"
		} else {
			r.Status = "installed"
		}
	case manifest.ServiceTailscale:
		out, err := Run(path, "status")
		r.Detail = firstLine(out)
		if err != nil {
			r.Status = "error"
			return r
		}
		low := strings.ToLower(out)
		if strings.Contains(low, "stopped") || strings.Contains(low, "logged out") {
			r.Status = "disconnected"
		} else {
			r.Status = "connected"
		}
	case manifest.ServiceOnePasswordAgent, manifest.ServiceSecretive:
		r.Status = "installed"
		r.Detail = "binary present; socket is not probed for a login"
	default:
		out, _ := Run(path, "--version")
		r.Version = firstLine(out)
		r.Status = "installed"
	}
	return r
}

// CheckAll reports every built-in kind plus names in defaults.services and
// those attached to hosts.
func CheckAll(m *manifest.Manifest) []Report {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		names = append(names, n)
	}
	for _, k := range manifest.BuiltinServiceKinds() {
		add(k)
	}
	if m != nil {
		for _, s := range m.Defaults.Services {
			add(s.Ref())
		}
		for _, pname := range m.ProfileNames() {
			for _, h := range m.Profiles[pname].Hosts {
				for _, n := range h.Services {
					add(n)
				}
				for _, v := range h.Variants {
					for _, n := range v.Services {
						add(n)
					}
				}
			}
		}
	}
	out := make([]Report, 0, len(names))
	for _, n := range names {
		out = append(out, Check(m, n))
	}
	return out
}

func binaryFor(m *manifest.Manifest, name, kind string) string {
	if m != nil {
		for _, s := range m.Defaults.Services {
			if s.Ref() == name && s.Binary != "" {
				return s.Binary
			}
		}
	}
	switch kind {
	case manifest.ServiceCloudflareAccess:
		return "cloudflared"
	case manifest.ServiceCloudflareWARP:
		return "warp-cli"
	case manifest.ServiceTailscale:
		return "tailscale"
	case manifest.ServiceTeleport:
		return "tsh"
	case manifest.ServiceAWSSSM:
		return "session-manager-plugin"
	case manifest.ServiceOnePasswordAgent:
		return "op"
	case manifest.ServiceSecretive:
		return "secretive"
	default:
		return ""
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
