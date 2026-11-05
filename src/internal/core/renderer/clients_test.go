package renderer

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf16"
)

func clientExport(t *testing.T, format string, opts TermiusOptions) *ClientExport {
	t.Helper()
	exp, err := RenderClientExport(mustManifest(t, exportJSON), testExportPaths, format, opts)
	if err != nil {
		t.Fatal(err)
	}
	return exp
}

// The generic CSV carries the key path, which Termius's does not, and splits
// tags on ; so a comma inside one survives.
func TestGenericCSVCarriesTheKeyPath(t *testing.T) {
	exp := clientExport(t, "csv", TermiusOptions{})
	rows := parseCSV(t, string(exp.Files[0].Data))
	if strings.Join(rows[0], ",") != strings.Join(GenericCSVColumns, ",") || len(rows) != 3 {
		t.Fatalf("rows: %q", rows)
	}
	box := rows[1]
	if box[0] != "box" || box[1] != "legacy" || box[3] != "2222" || box[5] != "/home/u/.ssh/profiles/legacy/legacy-ed25519" ||
		box[6] != "prod;eu,west" {
		t.Errorf("box = %q", box)
	}
	if s := strings.Join(exp.Skipped, "\n"); !strings.Contains(s, "devbox") || !strings.Contains(s, "github-imani") {
		t.Errorf("skipped = %v", exp.Skipped)
	}
}

// Windows Terminal and iTerm2 run `ssh <alias>`, so a VM host, reachable only
// through its ProxyCommand, is kept: OpenSSH supplies the route.
func TestTheSSHLaunchingExportsKeepProxiedHosts(t *testing.T) {
	wt := clientExport(t, "windows-terminal", TermiusOptions{})
	var doc struct {
		Profiles []struct{ Name, Commandline string }
	}
	if err := json.Unmarshal(wt.Files[0].Data, &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range doc.Profiles {
		got[p.Name] = p.Commandline
	}
	if got["simtabi / devbox"] != "ssh.exe devbox" || got["legacy / box"] != "ssh.exe box" || len(got) != 3 {
		t.Fatalf("profiles = %v", got)
	}

	it := clientExport(t, "iterm2", TermiusOptions{IncludeGit: true})
	var prof struct {
		Profiles []struct {
			Name, Guid, Command string
			Custom              string `json:"Custom Command"`
			Tags                []string
		}
	}
	if err := json.Unmarshal(it.Files[0].Data, &prof); err != nil {
		t.Fatal(err)
	}
	if len(prof.Profiles) != 4 {
		t.Fatalf("iterm2 profiles = %+v", prof.Profiles)
	}
	for _, p := range prof.Profiles {
		if p.Guid != "authama-"+p.Name || p.Command != "ssh "+p.Name || p.Custom != "Yes" || len(p.Tags) == 0 {
			t.Errorf("profile %+v", p)
		}
	}
}

// PuTTY's .reg is UTF-16LE with a BOM and CRLF, as regedit writes Version
// 5.00, points each session at the key's .ppk and says how to make it.
func TestPuTTYRegIsWhatRegeditReads(t *testing.T) {
	exp := clientExport(t, "putty", TermiusOptions{})
	data := exp.Files[0].Data
	if data[0] != 0xFF || data[1] != 0xFE || len(data)%2 != 0 {
		t.Fatal("not UTF-16LE with a byte order mark")
	}
	u := make([]uint16, (len(data)-2)/2)
	for i := range u {
		u[i] = uint16(data[2+2*i]) | uint16(data[3+2*i])<<8
	}
	text := string(utf16.Decode(u))
	for _, want := range []string{
		"Windows Registry Editor Version 5.00\r\n",
		`[HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions\box]`,
		`"PortNumber"=dword:000008ae`,
		`"PublicKeyFile"="/home/u/.ssh/profiles/legacy/legacy-ed25519.ppk"`,
		`"Folder"="legacy"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Error("a bare LF: every line must end CRLF")
	}
	if !strings.Contains(strings.Join(exp.Notes, "\n"), "puttygen /home/u/.ssh/profiles/legacy/legacy-ed25519 -o") {
		t.Errorf("no conversion hint: %v", exp.Notes)
	}
	if puttyName(`a b\c*%`) != `a%20b%5Cc%2A%25` {
		t.Errorf("session name escaping: %s", puttyName(`a b\c*%`))
	}
}

func TestRemminaWritesOneProfilePerHost(t *testing.T) {
	exp := clientExport(t, "remmina", TermiusOptions{})
	if len(exp.Files) != 2 {
		t.Fatalf("files = %d", len(exp.Files))
	}
	var box string
	for _, f := range exp.Files {
		if f.Name == "authama-box.remmina" {
			box = string(f.Data)
		}
	}
	for _, want := range []string{"[remmina]\n", "protocol=SSH\n", "server=box.example:2222\n", "username=me\n",
		"ssh_auth=1\n", "ssh_privatekey=/home/u/.ssh/profiles/legacy/legacy-ed25519\n", "group=legacy\n"} {
		if !strings.Contains(box, want) {
			t.Errorf("missing %q in:\n%s", want, box)
		}
	}
	if remminaValue(" a\\b\n") != `\sa\\b\n` {
		t.Errorf("escaping: %q", remminaValue(" a\\b\n"))
	}
}

func TestAnUnknownClientOrSkipIsAnError(t *testing.T) {
	m := mustManifest(t, exportJSON)
	if _, err := RenderClientExport(m, testExportPaths, "nope", TermiusOptions{}); err == nil {
		t.Error("an unknown client was accepted")
	}
	if _, err := RenderClientExport(m, testExportPaths, "csv", TermiusOptions{Skip: []string{"no-such"}}); err == nil {
		t.Error("a --skip naming no host was accepted")
	}
	exp, err := RenderClientExport(m, testExportPaths, "csv", TermiusOptions{Skip: []string{"box"}})
	if err != nil || strings.Contains(string(exp.Files[0].Data), "box.example") {
		t.Errorf("--skip box: %v", err)
	}
}
