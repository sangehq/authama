package renderer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/envnames"
)

// RouteDirect reports whether AUTHAMA_DEPLOY_ROUTE (ssh.deploy_route) asks for
// the direct route: deploy, verify and remove connect to the manifest's
// hostname, user and port instead of the host's alias.
func RouteDirect(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	return strings.EqualFold(strings.TrimSpace(envnames.Get(getenv, envnames.DeployRoute)), "direct")
}

// HasRoute reports whether ssh reaches h through something the managed config
// sets up - a cloudflared proxy, a VM's ProxyCommand, or a ProxyJump or
// ProxyCommand raw option - so a direct TCP probe of its hostname says
// nothing about whether ssh can reach it.
func HasRoute(h manifest.Host) bool {
	if h.Proxy != nil || h.VM != nil {
		return true
	}
	for _, k := range h.RawOptions.Keys() {
		switch strings.ToLower(k) {
		case "proxyjump", "proxycommand":
			return true
		}
	}
	return false
}

// RouteFileName is the route config's name in the state home.
const RouteFileName = "route.conf"

// RouteConfig writes the flat managed config (every host, every path
// absolute) to <stateDir>/route.conf, owner-only, and returns its path, for
// `ssh -F` to resolve a host's alias exactly as `ssh <alias>` does, with the
// jump hosts a ProxyJump names in it too. It returns "" under the direct route
// (ssh.deploy_route direct), or when the file cannot be written, and the
// caller then connects directly. A flat copy, rather than ~/.ssh/config
// itself, because a config given with -F reads its relative Includes from
// the real ~/.ssh, which under --dev-root is not the sandbox. It holds what
// `ssh config export` does: host names and paths, never a key.
func RouteConfig(m *manifest.Manifest, sshDir, stateDir string, emitUseKeychain, direct bool) string {
	if m == nil || direct || RouteDirect(nil) {
		return ""
	}
	text, err := RenderFlatConfig(m, ExportPathsFor(sshDir), emitUseKeychain)
	if err != nil {
		return ""
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return ""
	}
	f, err := os.CreateTemp(stateDir, ".route-*.tmp")
	if err != nil {
		return ""
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if runtime.GOOS != "windows" {
		if err := f.Chmod(0o600); err != nil {
			_ = f.Close()
			return ""
		}
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return ""
	}
	if err := f.Close(); err != nil {
		return ""
	}
	dst := filepath.Join(stateDir, RouteFileName)
	if err := os.Rename(tmp, dst); err != nil {
		return ""
	}
	ok = true
	return dst
}
