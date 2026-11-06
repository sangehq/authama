package renderer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/encode"
)

// Source is the manifest as the export pipeline's host formats read it
// (encode.HostSource): the hosts a selection kept, as a client sees them, or
// their managed ssh_config.
type Source struct {
	M               *manifest.Manifest
	P               ExportPaths
	EmitUseKeychain bool
}

var _ encode.HostSource = Source{}

// ClientHosts are the hosts named, in the order named, chosen as
// RenderClientExport chooses them: a host behind a ProxyCommand only for a
// client that runs ssh, a git host only with includeGit.
func (s Source) ClientHosts(aliases []string, client string, viaSSH, includeGit bool) ([]encode.Host, []string, error) {
	resolved, err := s.M.IterResolved()
	if err != nil {
		return nil, nil, err
	}
	byAlias := map[string]manifest.ResolvedKey{}
	for _, rk := range resolved {
		byAlias[rk.Host.Alias] = rk
	}
	var unknown []string
	for _, a := range aliases {
		if _, ok := byAlias[a]; !ok {
			unknown = append(unknown, a)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, nil, fmt.Errorf("no host named %s", strings.Join(unknown, ", "))
	}
	var hosts []encode.Host
	var skipped []string
	for _, a := range aliases {
		rk := byAlias[a]
		h := rk.Host
		switch {
		case !viaSSH && (h.VM != nil || h.Proxy != nil):
			skipped = append(skipped, h.Alias+": reached through a ProxyCommand, which "+client+" does not run")
			continue
		case isGitHost(h) && !includeGit:
			skipped = append(skipped, h.Alias+": a git host, not a shell (keep it with --include-git)")
			continue
		}
		hosts = append(hosts, clientHost(s.M, s.P, rk))
	}
	return hosts, skipped, nil
}

// SSHConfig is the flat export of the hosts named.
func (s Source) SSHConfig(aliases []string) (string, error) {
	return RenderFlatConfigOf(s.M, s.P, s.EmitUseKeychain, aliases)
}

// ManifestOrder is aliases in manifest order, as the client exports list
// hosts; an alias the manifest does not have keeps its place at the end.
func (s Source) ManifestOrder(aliases []string) ([]string, error) {
	resolved, err := s.M.IterResolved()
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, a := range aliases {
		want[a] = true
	}
	out := make([]string, 0, len(aliases))
	for _, rk := range resolved {
		if want[rk.Host.Alias] {
			out = append(out, rk.Host.Alias)
			delete(want, rk.Host.Alias)
		}
	}
	for _, a := range aliases {
		if want[a] {
			out = append(out, a)
		}
	}
	return out, nil
}

// clientHost is a resolved host as a client sees it: port 22 when none is
// set, the key as an absolute path, or the agent when the key is in 1Password.
func clientHost(m *manifest.Manifest, p ExportPaths, rk manifest.ResolvedKey) encode.Host {
	h := rk.Host
	port := h.Port
	if port == 0 {
		port = 22
	}
	file, agent := identityFor(m, rk.Profile, rk.KeyName)
	ch := encode.Host{Group: rk.Profile, Alias: h.Alias, Hostname: h.Hostname, User: h.User, Port: port, Tags: h.Tags, Agent: p.expand(agent)}
	if agent == "" {
		ch.KeyFile = p.expand(file)
	}
	return ch
}
