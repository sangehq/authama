package renderer

import (
	"encoding/xml"
	"strings"
	"testing"
)

// The SecureCRT file is well-formed XML SecureCRT's Import Settings reads: a
// folder per group, an SSH2 session per host with its own key.
func TestSecureCRTWritesAnImportableSettingsFile(t *testing.T) {
	exp := clientExport(t, "securecrt", TermiusOptions{})
	if len(exp.Files) != 1 || exp.Files[0].Name != "authama-securecrt.xml" {
		t.Fatalf("files = %+v", exp.Files)
	}
	type value struct {
		Name string `xml:"name,attr"`
		Text string `xml:",chardata"`
	}
	type key struct {
		Name    string  `xml:"name,attr"`
		Keys    []key   `xml:"key"`
		Strings []value `xml:"string"`
		Dwords  []value `xml:"dword"`
	}
	var doc struct {
		XMLName xml.Name `xml:"VanDyke"`
		Keys    []key    `xml:"key"`
	}
	if err := xml.Unmarshal(exp.Files[0].Data, &doc); err != nil {
		t.Fatalf("not XML: %v\n%s", err, exp.Files[0].Data)
	}
	if len(doc.Keys) != 1 || doc.Keys[0].Name != "Sessions" {
		t.Fatalf("top keys = %+v", doc.Keys)
	}
	var box *key
	for gi := range doc.Keys[0].Keys {
		for hi := range doc.Keys[0].Keys[gi].Keys {
			if doc.Keys[0].Keys[gi].Keys[hi].Name == "box" {
				if doc.Keys[0].Keys[gi].Name != "legacy" {
					t.Errorf("box is in folder %q", doc.Keys[0].Keys[gi].Name)
				}
				box = &doc.Keys[0].Keys[gi].Keys[hi]
			}
		}
	}
	if box == nil {
		t.Fatal("no box session")
	}
	got := map[string]string{}
	for _, v := range append(box.Strings, box.Dwords...) {
		got[v.Name] = v.Text
	}
	for name, want := range map[string]string{"Protocol Name": "SSH2", "Hostname": "box.example", "[SSH2] Port": "2222",
		"Username": "me", "Use Global Public Key": "0", "Identity Filename V2": "/home/u/.ssh/profiles/legacy/legacy-ed25519"} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	// Markup in a value stays a value.
	if xmlText(`a<b&"c`) != "a&lt;b&amp;&#34;c" {
		t.Errorf("escaping: %q", xmlText(`a<b&"c`))
	}
}

// The MobaXterm file is a .mxtsessions INI: a bookmark folder per group and
// one SSH session line per host, in MobaXterm's own field order.
func TestMobaXtermWritesAnImportableSessionsFile(t *testing.T) {
	exp := clientExport(t, "mobaxterm", TermiusOptions{})
	if len(exp.Files) != 1 || exp.Files[0].Name != "authama.mxtsessions" {
		t.Fatalf("files = %+v", exp.Files)
	}
	text := string(exp.Files[0].Data)
	if !strings.HasPrefix(text, "[Bookmarks]\r\n") || !strings.Contains(text, "SubRep=legacy\r\n") {
		t.Fatalf("no folders:\n%s", text)
	}
	var line string
	for _, l := range strings.Split(text, "\r\n") {
		if strings.HasPrefix(l, "box=") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no box session:\n%s", text)
	}
	session := strings.Split(strings.TrimPrefix(line, "box="), "#")
	if len(session) < 3 || session[1] != "109" {
		t.Fatalf("not an SSH session: %q", line)
	}
	f := strings.Split(session[2], "%")
	if f[1] != "box.example" || f[2] != "2222" || f[3] != "me" || f[14] != "/home/u/.ssh/profiles/legacy/legacy-ed25519" {
		t.Errorf("fields = %q", f)
	}
	// A separator in a value cannot shift the fields.
	if got := mobaField("a%b#c=d\ne"); got != "a_b_c-d e" {
		t.Errorf("mobaField = %q", got)
	}
}
