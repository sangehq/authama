package encode

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// The SSH clients' import formats. They write hosts, not a table: each client
// reads a host, port, user and key, and most want files of their own shape.
// They moved here from core/renderer, which still offers them by their old
// names and now delegates (RenderClientExport, RenderTermiusCSV), so the export
// pipeline and `ssh host export` write the same bytes.

// Host is one host as an SSH client's import sees it: the profile as its
// group, the alias as its name, and the key as an absolute path.
type Host struct {
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

// Client is one SSH client's import format.
type Client struct {
	Name, Client string
	// ViaSSH formats launch `ssh <alias>`, so OpenSSH reads the managed config
	// and a ProxyCommand or VM host works as it does at the terminal; the
	// others hand the client a host, port, user and key and keep only hosts
	// the client can reach on its own.
	ViaSSH bool
	Render func(hosts []Host) (*ClientExport, error)
}

// Clients are the client formats `ssh host export` names, by name.
var Clients = map[string]Client{
	"csv":              {Name: "csv", Client: "any client that imports a CSV (Royal TS, Xshell, SecureCRT's import script, a spreadsheet)", Render: renderGenericCSV},
	"windows-terminal": {Name: "windows-terminal", Client: "Windows Terminal (a JSON fragment)", ViaSSH: true, Render: renderWindowsTerminal},
	"iterm2":           {Name: "iterm2", Client: "iTerm2 (Dynamic Profiles)", ViaSSH: true, Render: renderITerm2},
	"putty":            {Name: "putty", Client: "PuTTY and KiTTY (a .reg file)", Render: renderPuTTY},
	"remmina":          {Name: "remmina", Client: "Remmina (one .remmina file per host)", Render: renderRemmina},
	"securecrt": {Name: "securecrt",
		Client: "SecureCRT (an XML settings file for File > Import Settings; Termius imports it too)", Render: renderSecureCRT},
	"mobaxterm": {Name: "mobaxterm",
		Client: "MobaXterm (a .mxtsessions file for Sessions > Import; Termius imports it too)", Render: renderMobaXterm},
}

// ClientNames are the client format names, sorted.
func ClientNames() []string {
	out := make([]string, 0, len(Clients))
	for n := range Clients {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// GenericCSVColumns are the generic export's columns.
var GenericCSVColumns = []string{"alias", "group", "hostname", "port", "user", "key_path", "tags"}

// TermiusColumns is the header of Termius's CSV import template, as the
// third-party exporters written against it reproduce it (Termius 9.x). Termius
// publishes the template only as a download inside the app, so this is the
// one place to change if it moves.
var TermiusColumns = []string{"Groups", "Label", "Tags", "Hostname/IP", "Protocol", "Port", "Username", "Password"}

// TermiusCSV is a Termius CSV import of hosts, one row each: group = profile,
// label = alias, tags = the host's tags. No password is ever written: the
// column is part of the template and stays empty.
func TermiusCSV(hosts []Host) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(TermiusColumns); err != nil {
		return "", err
	}
	for _, h := range hosts {
		row := []string{
			cell(h.Group), cell(h.Alias), termiusTags(h.Tags),
			cell(h.Hostname), "ssh", strconv.Itoa(h.Port), cell(h.User), "",
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// termiusTags joins a host's tags into one field. A comma inside a tag would
// read back as two tags, so it becomes a space.
func termiusTags(tags []string) string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, cell(strings.ReplaceAll(t, ",", " ")))
	}
	return strings.Join(out, ",")
}

// cell neutralises a value a spreadsheet would evaluate as a formula. The CSV
// is meant for an SSH client, but a CSV is opened in a spreadsheet often
// enough that one free-text field starting with = must not become a live
// formula there.
func cell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func renderGenericCSV(hosts []Host) (*ClientExport, error) {
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
func renderWindowsTerminal(hosts []Host) (*ClientExport, error) {
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
func renderITerm2(hosts []Host) (*ClientExport, error) {
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
func renderPuTTY(hosts []Host) (*ClientExport, error) {
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
func renderRemmina(hosts []Host) (*ClientExport, error) {
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

// groups orders hosts by group, keeping each group's hosts in manifest order.
func groups(hosts []Host) (names []string, by map[string][]Host) {
	by = map[string][]Host{}
	for _, h := range hosts {
		if _, ok := by[h.Group]; !ok {
			names = append(names, h.Group)
		}
		by[h.Group] = append(by[h.Group], h)
	}
	sort.Strings(names)
	return names, by
}

// xmlText escapes s for an XML attribute or element.
func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return strings.ReplaceAll(b.String(), `"`, "&#34;")
}

// renderSecureCRT writes SecureCRT's exported-settings XML: a Sessions key
// with a folder per group and an SSH2 session per host, each naming its key
// as the session's own identity file. SecureCRT 8 and later read OpenSSH
// keys, so the key needs no conversion.
func renderSecureCRT(hosts []Host) (*ClientExport, error) {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<VanDyke version=\"3.0\">\n\t<key name=\"Sessions\">\n")
	names, by := groups(hosts)
	for _, g := range names {
		fmt.Fprintf(&b, "\t\t<key name=\"%s\">\n", xmlText(g))
		for _, h := range by[g] {
			fmt.Fprintf(&b, "\t\t\t<key name=\"%s\">\n", xmlText(h.Alias))
			fmt.Fprintf(&b, "\t\t\t\t<string name=\"Protocol Name\">SSH2</string>\n")
			fmt.Fprintf(&b, "\t\t\t\t<string name=\"Hostname\">%s</string>\n", xmlText(h.Hostname))
			fmt.Fprintf(&b, "\t\t\t\t<dword name=\"[SSH2] Port\">%d</dword>\n", h.Port)
			fmt.Fprintf(&b, "\t\t\t\t<string name=\"Username\">%s</string>\n", xmlText(h.User))
			b.WriteString("\t\t\t\t<string name=\"SSH2 Authentications V2\">publickey,keyboard-interactive,password</string>\n")
			if h.KeyFile != "" {
				// The session's own key, not SecureCRT's global one.
				b.WriteString("\t\t\t\t<dword name=\"Use Global Public Key\">0</dword>\n")
				fmt.Fprintf(&b, "\t\t\t\t<string name=\"Identity Filename V2\">%s</string>\n", xmlText(h.KeyFile))
			}
			if len(h.Tags) > 0 {
				fmt.Fprintf(&b, "\t\t\t\t<string name=\"Description\">%s</string>\n", xmlText(strings.Join(h.Tags, ", ")))
			}
			b.WriteString("\t\t\t</key>\n")
		}
		b.WriteString("\t\t</key>\n")
	}
	b.WriteString("\t</key>\n</VanDyke>\n")
	return &ClientExport{
		Files: []ClientFile{{Name: "authama-securecrt.xml", Data: []byte(b.String())}},
		Notes: []string{"In SecureCRT, File > Import Settings and choose authama-securecrt.xml (Termius: Import > SecureCRT takes the same file). " +
			"Key paths are this machine's; edit them if SecureCRT runs elsewhere. A host whose key is in 1Password uses the agent."},
	}, nil
}

// mobaField keeps a value out of MobaXterm's separators: % splits a
// session's fields and # its sections, and a line break ends the entry.
func mobaField(s string) string {
	return strings.NewReplacer("%", "_", "#", "_", "\r", " ", "\n", " ", "=", "-").Replace(s)
}

// renderMobaXterm writes a .mxtsessions file: a bookmark folder per group and
// an SSH session per host, in MobaXterm's own session string, which carries
// host, port, user and private key. MobaXterm reads OpenSSH keys.
func renderMobaXterm(hosts []Host) (*ClientExport, error) {
	var b strings.Builder
	b.WriteString("[Bookmarks]\r\nSubRep=\r\nImgNum=42\r\n")
	names, by := groups(hosts)
	for i, g := range names {
		fmt.Fprintf(&b, "\r\n[Bookmarks_%d]\r\nSubRep=%s\r\nImgNum=41\r\n", i+1, mobaField(g))
		for _, h := range by[g] {
			// #109 is an SSH session; the fields after it are host, port, user,
			// then MobaXterm's defaults, with the private key in the 14th.
			fmt.Fprintf(&b, "%s=#109#0%%%s%%%d%%%s%%%%-1%%-1%%%%%%22%%%%0%%0%%0%%%s%%%%-1%%0%%0%%0%%%%1080%%%%0%%0%%1#MobaFont%%10%%0%%0%%-1%%15%%236,236,236%%30,30,30%%180,180,192%%0%%-1%%0%%%%xterm%%-1%%-1%%_Std_Colors_0_%%80%%24%%0%%1%%-1%%<none>%%%%0%%0%%-1#0#%s#-1\r\n",
				mobaField(h.Alias), mobaField(h.Hostname), h.Port, mobaField(h.User), mobaField(h.KeyFile), mobaField(strings.Join(h.Tags, " ")))
		}
	}
	return &ClientExport{
		Files: []ClientFile{{Name: "authama.mxtsessions", Data: []byte(b.String())}},
		Notes: []string{"In MobaXterm, Sessions > Import sessions from file and choose authama.mxtsessions (Termius: Import > MobaXterm takes the same file). " +
			"Key paths are this machine's; on Windows, point each session at the key's copy there."},
	}, nil
}
