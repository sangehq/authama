package renderer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/encode"
)

// ClientHost is one host as an SSH client's import sees it. It is
// encode.Host: the writers moved to util/encode, where the export pipeline
// uses them too.
type ClientHost = encode.Host

// ClientFile is one file an export writes: its name and its bytes.
type ClientFile = encode.ClientFile

// ClientExport is a rendered export: the files, the hosts left out with the
// reason for each, and notes the user needs to finish the import.
type ClientExport = encode.ClientExport

// ClientFormat is one SSH client's import format.
type ClientFormat struct {
	Name, Client string
	// ViaSSH formats launch `ssh <alias>`, so OpenSSH reads the managed config
	// and a ProxyCommand or VM host works as it does at the terminal; the
	// others hand the client a host, port, user and key and keep only hosts
	// the client can reach on its own.
	ViaSSH bool
	render func(hosts []ClientHost) (*ClientExport, error)
}

// ClientFormats are the exports, by name: encode.Clients, whose writers they
// call.
var ClientFormats = func() map[string]ClientFormat {
	out := map[string]ClientFormat{}
	for n, c := range encode.Clients {
		out[n] = ClientFormat{Name: c.Name, Client: c.Client, ViaSSH: c.ViaSSH, render: c.Render}
	}
	return out
}()

// ClientFormatNames are the export names, sorted.
func ClientFormatNames() []string {
	out := make([]string, 0, len(ClientFormats))
	for n := range ClientFormats {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// RenderClientExport renders the manifest's hosts for one SSH client. The
// hosts are chosen as for Termius (see RenderTermiusCSV): git hosts only with
// IncludeGit, never a --skip alias, and, for a format that does not go
// through ssh, never a host reached through a ProxyCommand.
func RenderClientExport(m *manifest.Manifest, p ExportPaths, format string, opts TermiusOptions) (*ClientExport, error) {
	f, ok := ClientFormats[format]
	if !ok {
		return nil, fmt.Errorf("unknown export %q (choose from %s)", format, strings.Join(ClientFormatNames(), ", "))
	}
	resolved, err := m.IterResolved()
	if err != nil {
		return nil, err
	}
	skip := map[string]bool{}
	for _, a := range opts.Skip {
		skip[a] = true
	}
	var hosts []ClientHost
	var skipped []string
	for _, rk := range resolved {
		h := rk.Host
		switch {
		case skip[h.Alias]:
			skipped = append(skipped, h.Alias+": --skip")
			delete(skip, h.Alias)
			continue
		case !f.ViaSSH && (h.VM != nil || h.Proxy != nil):
			skipped = append(skipped, h.Alias+": reached through a ProxyCommand, which "+f.Client+" does not run; use windows-terminal or iterm2, which go through ssh")
			continue
		case isGitHost(h) && !opts.IncludeGit:
			skipped = append(skipped, h.Alias+": a git host, not a shell (export it with --include-git)")
			continue
		}
		hosts = append(hosts, clientHost(m, p, rk))
	}
	if len(skip) > 0 {
		var unknown []string
		for a := range skip {
			unknown = append(unknown, a)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("--skip names no host: %s", strings.Join(unknown, ", "))
	}
	exp, err := f.render(hosts)
	if err != nil {
		return nil, err
	}
	exp.Skipped = skipped
	return exp, nil
}

// GenericCSVColumns are the generic export's columns (encode.GenericCSVColumns).
var GenericCSVColumns = encode.GenericCSVColumns
