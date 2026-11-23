package svchealth

import (
	"errors"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

func TestCheckUsesReadOnlySeams(t *testing.T) {
	origPath, origRun := LookPath, Run
	t.Cleanup(func() { LookPath, Run = origPath, origRun })
	LookPath = func(name string) (string, error) {
		if name == "warp-cli" {
			return "/usr/bin/warp-cli", nil
		}
		if name == "cloudflared" {
			return "/usr/local/bin/cloudflared", nil
		}
		return "", errors.New("not found")
	}
	var calls []string
	Run = func(name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if strings.Contains(name, "warp-cli") {
			return "Status update: Connected\nReason: happy", nil
		}
		return "cloudflared version 2026.11.2", nil
	}
	w := Check(nil, manifest.ServiceCloudflareWARP)
	if !w.Installed || w.Status != "connected" {
		t.Fatalf("warp: %+v", w)
	}
	c := Check(nil, manifest.ServiceCloudflareAccess)
	if c.Status != "installed" || c.Version == "" {
		t.Fatalf("access: %+v", c)
	}
	miss := Check(nil, manifest.ServiceTailscale)
	if miss.Installed || miss.Status != "not installed" {
		t.Fatalf("tailscale: %+v", miss)
	}
	for _, call := range calls {
		if strings.Contains(call, "connect") && !strings.Contains(call, "status") {
			t.Errorf("health must stay read-only: %s", call)
		}
	}
}
