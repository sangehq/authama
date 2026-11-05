package renderer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/simtabi/authama/src/internal/core/manifest"
)

// ClientHost is one host as an SSH client's import sees it: the profile as
// its group, the alias as its name, and the key as an absolute path.
type ClientHost struct {
	Group, Alias, Hostname, User string
	Port                         int
	Tags                         []string
	KeyFile                      string // absolute; "" when the key is in 1Password
	Agent                        string // the 1Password agent socket, when it is
}

// ClientFile is one file an export writes: its name and its bytes.
type ClientFile struct {
	Name string
	Data []byte
}

// ClientExport is a rendered export: the files, the hosts left out with the
// reason for each, and notes the user needs to finish the import.
type ClientExport struct {
	Files   []ClientFile
	Skipped []string
	Notes   []string
}

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

// ClientFormats are the exports, by name.
var ClientFormats = map[string]ClientFormat{
	"csv":              {Name: "csv", Client: "any client that imports a CSV (Royal TS, Xshell, SecureCRT's import script, a spreadsheet)", render: renderGenericCSV},
	"windows-terminal": {Name: "windows-terminal", Client: "Windows Terminal (a JSON fragment)", ViaSSH: true, render: renderWindowsTerminal},
	"iterm2":           {Name: "iterm2", Client: "iTerm2 (Dynamic Profiles)", ViaSSH: true, render: renderITerm2},
	"putty":            {Name: "putty", Client: "PuTTY and KiTTY (a .reg file)", render: renderPuTTY},
	"remmina":          {Name: "remmina", Client: "Remmina (one .remmina file per host)", render: renderRemmina},
}

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
		port := h.Port
		if port == 0 {
			port = 22
		}
		file, agent := identityFor(m, rk.Profile, rk.KeyName)
		ch := ClientHost{Group: rk.Profile, Alias: h.Alias, Hostname: h.Hostname, User: h.User, Port: port, Tags: h.Tags, Agent: p.expand(agent)}
		if agent == "" {
			ch.KeyFile = p.expand(file)
		}
		hosts = append(hosts, ch)
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

// GenericCSVColumns are the generic export's columns.
var GenericCSVColumns = []string{"alias", "group", "hostname", "port", "user", "key_path", "tags"}

func renderGenericCSV(hosts []ClientHost) (*ClientExport, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(GenericCSVColumns); err != nil {
		return nil, err
	}
	for _, h := range hosts {
		tags := make([]string, 0, len(h.Tags))
		for _, t := range h.Tags {
			tags = append(tags, strings.ReplaceAll(t, ";", " "))
		}
		if err := w.Write([]string{cell(h.Alias), cell(h.Group), cell(h.Hostname), strconv.Itoa(h.Port), cell(h.User),
			cell(h.KeyFile), cell(strings.Join(tags, ";"))}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return &ClientExport{
		Files: []ClientFile{{Name: "hosts.csv", Data: buf.Bytes()}},
		Notes: []string{"Royal TS maps the host to its URI column, not hostname; Royal TS's and Xshell's CSV imports do not take key_path, so attach each key in the client."},
	}, nil
}

// renderWindowsTerminal writes a JSON fragment: one profile per host, each
// running `ssh.exe <alias>`, named "group / alias" because profiles have no
// folders. Windows Terminal derives each profile's GUID from its name, so a
// re-export updates the same profiles.
func renderWindowsTerminal(hosts []ClientHost) (*ClientExport, error) {
	type profile struct {
		Name        string `json:"name"`
		Commandline string `json:"commandline"`
	}
	doc := struct {
		Profiles []profile `json:"profiles"`
	}{Profiles: []profile{}}
	for _, h := range hosts {
		doc.Profiles = append(doc.Profiles, profile{Name: h.Group + " / " + h.Alias, Commandline: "ssh.exe " + h.Alias})
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return &ClientExport{
		Files: []ClientFile{{Name: "authama.json", Data: append(b, '\n')}},
		Notes: []string{`Copy authama.json to %LOCALAPPDATA%\Microsoft\Windows Terminal\Fragments\authama\ and restart Windows Terminal. Each profile runs ssh, which reads your ~/.ssh/config, so keys and ProxyCommands work as at the terminal.`},
	}, nil
}

// renderITerm2 writes Dynamic Profiles: one profile per host running
// `ssh <alias>`, tagged with its group and tags. The GUID is fixed per alias
// so a re-export updates the profiles rather than adding new ones.
func renderITerm2(hosts []ClientHost) (*ClientExport, error) {
	type profile struct {
		Name          string   `json:"Name"`
		GUID          string   `json:"Guid"`
		CustomCommand string   `json:"Custom Command"`
		Command       string   `json:"Command"`
		Tags          []string `json:"Tags"`
	}
	doc := struct {
		Profiles []profile `json:"Profiles"`
	}{Profiles: []profile{}}
	for _, h := range hosts {
		doc.Profiles = append(doc.Profiles, profile{Name: h.Alias, GUID: "authama-" + h.Alias, CustomCommand: "Yes",
			Command: "ssh " + h.Alias, Tags: append([]string{h.Group}, h.Tags...)})
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return &ClientExport{
		Files: []ClientFile{{Name: "authama-hosts.json", Data: append(b, '\n')}},
		Notes: []string{"Copy authama-hosts.json to ~/Library/Application Support/iTerm2/DynamicProfiles/; iTerm2 loads it at once. Each profile runs ssh, which reads your ~/.ssh/config."},
	}, nil
}

// puttyName escapes a session name as PuTTY stores it in the registry: a
// space, \, *, ?, % or anything outside printable ASCII becomes %XX.
func puttyName(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c <= ' ' || c > '~' || strings.IndexByte(`\*?%`, c) >= 0 {
			fmt.Fprintf(&b, "%%%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func regString(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// renderPuTTY writes a .reg file of PuTTY sessions, UTF-16LE with a byte
// order mark and CRLF line endings, as regedit writes a Version 5.00 file.
// PuTTY reads only its own .ppk keys, so each session names <key>.ppk, and
// the notes say how to convert each key.
func renderPuTTY(hosts []ClientHost) (*ClientExport, error) {
	var b strings.Builder
	b.WriteString("Windows Registry Editor Version 5.00\r\n")
	var convert []string
	for _, h := range hosts {
		fmt.Fprintf(&b, "\r\n[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\%s]\r\n", puttyName(h.Alias))
		fmt.Fprintf(&b, "\"HostName\"=\"%s\"\r\n", regString(h.Hostname))
		fmt.Fprintf(&b, "\"PortNumber\"=dword:%08x\r\n", h.Port)
		fmt.Fprintf(&b, "\"UserName\"=\"%s\"\r\n", regString(h.User))
		b.WriteString("\"Protocol\"=\"ssh\"\r\n")
		fmt.Fprintf(&b, "\"Folder\"=\"%s\"\r\n", regString(h.Group))
		if h.KeyFile != "" {
			fmt.Fprintf(&b, "\"PublicKeyFile\"=\"%s\"\r\n", regString(h.KeyFile+".ppk"))
			convert = append(convert, "puttygen "+h.KeyFile+" -o "+h.KeyFile+".ppk")
		}
	}
	u := utf16.Encode([]rune(b.String()))
	data := make([]byte, 2, 2+2*len(u))
	data[0], data[1] = 0xFF, 0xFE
	for _, c := range u {
		data = append(data, byte(c), byte(c>>8))
	}
	notes := []string{"Double-click authama-putty.reg (or `reg import authama-putty.reg`) on the Windows machine. Key paths are this machine's; edit them if PuTTY runs elsewhere. KiTTY also reads each session's Folder."}
	if len(convert) > 0 {
		notes = append(notes, "PuTTY reads only .ppk keys; convert each once:")
		notes = append(notes, convert...)
	}
	return &ClientExport{Files: []ClientFile{{Name: "authama-putty.reg", Data: data}}, Notes: notes}, nil
}

// remminaValue is a GLib key file value: a backslash, newline, tab or a
// leading space is escaped.
func remminaValue(s string) string {
	s = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`).Replace(s)
	if strings.HasPrefix(s, " ") {
		s = `\s` + s[1:]
	}
	return s
}

// renderRemmina writes one .remmina file per host: protocol SSH, the key file
// as the identity (ssh_auth=1 with ssh_privatekey), the group as Remmina's
// group.
func renderRemmina(hosts []ClientHost) (*ClientExport, error) {
	exp := &ClientExport{Notes: []string{"Copy the .remmina files to ~/.local/share/remmina/ (or import them from Remmina's menu). Remmina also wants the key's .pub beside it, which authama always keeps."}}
	for _, h := range hosts {
		var b strings.Builder
		b.WriteString("[remmina]\n")
		fmt.Fprintf(&b, "name=%s\n", remminaValue(h.Alias))
		fmt.Fprintf(&b, "group=%s\n", remminaValue(h.Group))
		b.WriteString("protocol=SSH\n")
		fmt.Fprintf(&b, "server=%s\n", remminaValue(h.Hostname+":"+strconv.Itoa(h.Port)))
		fmt.Fprintf(&b, "username=%s\n", remminaValue(h.User))
		if h.KeyFile != "" {
			b.WriteString("ssh_auth=1\n")
			fmt.Fprintf(&b, "ssh_privatekey=%s\n", remminaValue(h.KeyFile))
		} else {
			b.WriteString("ssh_auth=2\n") // the agent, for a key kept in 1Password
		}
		exp.Files = append(exp.Files, ClientFile{Name: "authama-" + fileSafe(h.Alias) + ".remmina", Data: []byte(b.String())})
	}
	return exp, nil
}

// fileSafe keeps a host alias usable as part of a file name.
func fileSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r < ' ' {
			return '_'
		}
		return r
	}, s)
}
