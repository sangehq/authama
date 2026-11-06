package renderer

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

func init() {
	ClientFormats["securecrt"] = ClientFormat{Name: "securecrt",
		Client: "SecureCRT (an XML settings file for File > Import Settings; Termius imports it too)", render: renderSecureCRT}
	ClientFormats["mobaxterm"] = ClientFormat{Name: "mobaxterm",
		Client: "MobaXterm (a .mxtsessions file for Sessions > Import; Termius imports it too)", render: renderMobaXterm}
}

// groups orders hosts by group, keeping each group's hosts in manifest order.
func groups(hosts []ClientHost) (names []string, by map[string][]ClientHost) {
	by = map[string][]ClientHost{}
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
func renderSecureCRT(hosts []ClientHost) (*ClientExport, error) {
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
func renderMobaXterm(hosts []ClientHost) (*ClientExport, error) {
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
