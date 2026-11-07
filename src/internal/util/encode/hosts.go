package encode

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// Host formats write host rows as an SSH client or ssh itself imports them,
// not as a table: the managed ssh_config of the chosen hosts, a Termius CSV,
// or one of the client formats (Clients). They read the hosts from a
// HostSource, which the caller builds over the manifest, so this package
// stays free of the domain.

// HostSource is what a host format reads.
type HostSource interface {
	// ClientHosts are the hosts named, as a client sees them. A host the
	// client cannot reach on its own (behind a ProxyCommand, when viaSSH is
	// false) or a git host (unless includeGit) is left out, with the reason
	// in skipped. client names the client in that reason.
	ClientHosts(aliases []string, client string, viaSSH, includeGit bool) (hosts []Host, skipped []string, err error)
	// SSHConfig is the managed config of exactly the hosts named, as
	// `authama ssh config export --flat` writes it for every host.
	SSHConfig(aliases []string) (string, error)
}

// ErrNotHosts is a host format asked to write a table.
var ErrNotHosts = errors.New("encode: a host format writes host rows, not a table")

// ClientCSV is the generic client CSV's name in the pipeline, where "csv" is
// the table CSV.
const ClientCSV = "hosts-csv"

func init() {
	register(Format{Name: "ssh_config", Extension: "", MediaType: "text/plain; charset=utf-8", Label: "ssh_config (OpenSSH)",
		Write: notTable, Hosts: writeSSHConfig})
	register(Format{Name: "termius", Extension: "csv", MediaType: "text/csv; charset=utf-8", Label: "Termius CSV",
		Write: notTable, Hosts: writeTermius})
	labels := map[string]string{
		"csv": "Client CSV (Royal TS, Xshell)", "windows-terminal": "Windows Terminal profiles", "iterm2": "iTerm2 profiles",
		"putty": "PuTTY sessions (.reg)", "remmina": "Remmina profiles (.zip)", "securecrt": "SecureCRT sessions",
		"mobaxterm": "MobaXterm sessions",
	}
	exts := map[string]string{
		"csv": "csv", "windows-terminal": "json", "iterm2": "json", "putty": "reg", "remmina": "zip",
		"securecrt": "xml", "mobaxterm": "mxtsessions",
	}
	media := map[string]string{
		"csv": "text/csv; charset=utf-8", "windows-terminal": "application/json; charset=utf-8",
		"iterm2": "application/json; charset=utf-8", "putty": "application/octet-stream", "remmina": "application/zip",
		"securecrt": "application/xml; charset=utf-8", "mobaxterm": "text/plain; charset=utf-8",
	}
	for name, c := range Clients {
		pipelineName := name
		if name == "csv" {
			pipelineName = ClientCSV
		}
		register(Format{Name: pipelineName, Extension: exts[name], MediaType: media[name], Label: labels[name],
			Client: name, Write: notTable, Hosts: clientWriter(c)})
	}
}

func notTable(io.Writer, Table, Options) error { return ErrNotHosts }

// clientWriter is a client format as a host format: the source's hosts,
// rendered by the client's own writer in Clients.
func clientWriter(c Client) func(HostSource, []string, Options) (*ClientExport, error) {
	return func(src HostSource, aliases []string, o Options) (*ClientExport, error) {
		hosts, skipped, err := src.ClientHosts(aliases, c.Client, c.ViaSSH, o.IncludeGit)
		if err != nil {
			return nil, err
		}
		exp, err := c.Render(hosts)
		if err != nil {
			return nil, err
		}
		exp.Skipped = skipped
		return exp, nil
	}
}

// writeSSHConfig is the managed config of the chosen hosts: byte for byte
// what `ssh config export --flat` writes when every host is chosen. Git
// hosts and hosts behind a ProxyCommand stay, since ssh itself reads it.
func writeSSHConfig(src HostSource, aliases []string, _ Options) (*ClientExport, error) {
	text, err := src.SSHConfig(aliases)
	if err != nil {
		return nil, err
	}
	return &ClientExport{
		Files: []ClientFile{{Name: "ssh_config", Data: []byte(text)}},
		Notes: []string{"ssh reads this file only when pointed at it: ssh -F ssh_config <alias>, or Include it from ~/.ssh/config."},
	}, nil
}

// writeTermius is the Termius CSV import of the chosen hosts. The CSV carries
// no keys, so the notes name the key to attach to each host.
func writeTermius(src HostSource, aliases []string, o Options) (*ClientExport, error) {
	hosts, skipped, err := src.ClientHosts(aliases, "Termius", false, o.IncludeGit)
	if err != nil {
		return nil, err
	}
	csv, err := TermiusCSV(hosts)
	if err != nil {
		return nil, err
	}
	notes := []string{"In Termius: Hosts > New Host (arrow) > Import > CSV. The CSV carries no keys; attach each in Termius (authama ssh termius keys <HOST> copies one):"}
	for _, h := range hosts {
		where := h.KeyFile
		if h.Agent != "" {
			where = "in 1Password: add it from 1Password, not from disk"
		}
		notes = append(notes, "  "+h.Alias+"  "+where)
	}
	return &ClientExport{Files: []ClientFile{{Name: "termius-hosts.csv", Data: []byte(csv)}}, Skipped: skipped, Notes: notes}, nil
}

// IsHosts reports a host format.
func (f Format) IsHosts() bool { return f.Hosts != nil }

// HostNames lists the host formats, sorted.
func HostNames() []string {
	var out []string
	for n, f := range registry {
		if f.IsHosts() {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// TableNames lists the table formats, sorted.
func TableNames() []string {
	var out []string
	for n, f := range registry {
		if !f.IsHosts() {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Bundle is an export as one file: its only file as it is, or, for a format
// that writes a file per host (its extension is zip), every file in a zip
// under its own name.
func Bundle(f Format, exp *ClientExport) ([]byte, error) {
	if f.Extension != "zip" {
		if len(exp.Files) != 1 {
			return nil, fmt.Errorf("encode: %s wrote %d files", f.Name, len(exp.Files))
		}
		return exp.Files[0].Data, nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range exp.Files {
		if f.Name != filepath.Base(f.Name) || strings.ContainsAny(f.Name, `\:`) {
			return nil, fmt.Errorf("encode: unsafe file name %q", f.Name)
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.Data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FileName is what a host format's file is called: the client's own name
// for it (authama-putty.reg), or the zip's.
func FileName(f Format, exp *ClientExport, base string) string {
	if f.Extension != "zip" && len(exp.Files) == 1 {
		return exp.Files[0].Name
	}
	return base + "." + f.Extension
}
