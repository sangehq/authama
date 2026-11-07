package encode

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeSource stands in for the manifest: it keeps or skips hosts by the same
// two rules the real one applies, and records what it was asked.
type fakeSource struct {
	hosts            map[string]Host
	proxied, git     map[string]bool
	asked            []string
	askedClient      string
	askedViaSSH, inG bool
}

func (f *fakeSource) ClientHosts(aliases []string, client string, viaSSH, includeGit bool) ([]Host, []string, error) {
	f.asked, f.askedClient, f.askedViaSSH, f.inG = aliases, client, viaSSH, includeGit
	var out []Host
	var skipped []string
	for _, a := range aliases {
		switch {
		case f.proxied[a] && !viaSSH:
			skipped = append(skipped, a+": proxied")
		case f.git[a] && !includeGit:
			skipped = append(skipped, a+": git")
		default:
			out = append(out, f.hosts[a])
		}
	}
	return out, skipped, nil
}

func (f *fakeSource) SSHConfig(aliases []string) (string, error) {
	f.asked = aliases
	return "Host " + strings.Join(aliases, "\nHost ") + "\n", nil
}

func newFake() *fakeSource {
	return &fakeSource{
		hosts: map[string]Host{
			"web": {Group: "acme", Alias: "web", Hostname: "203.0.113.5", User: "deploy", Port: 22, Tags: []string{"prd"}, KeyFile: "/k/web"},
			"vm":  {Group: "acme", Alias: "vm", Hostname: "vm", User: "me", Port: 22},
			"gh":  {Group: "me", Alias: "gh", Hostname: "github.com", User: "git", Port: 22, KeyFile: "/k/gh"},
		},
		proxied: map[string]bool{"vm": true},
		git:     map[string]bool{"gh": true},
	}
}

// A host format refuses a table: it writes hosts, read from a source.
func TestAHostFormatRefusesATable(t *testing.T) {
	for _, n := range HostNames() {
		f, _ := Get(n)
		if f.MediaType == "" || f.Label == "" {
			t.Errorf("%s lacks a media type or label", n)
		}
		if err := f.Write(&bytes.Buffer{}, sample(), Options{}); !errors.Is(err, ErrNotHosts) {
			t.Errorf("%s wrote a table: %v", n, err)
		}
	}
}

// Each client format writes with the client's own writer, the one `ssh host
// export` uses, and asks the source with the client's own rules.
func TestEveryClientFormatUsesItsClientsWriter(t *testing.T) {
	for name, c := range Clients {
		pn := name
		if name == "csv" {
			pn = ClientCSV
		}
		f, err := Get(pn)
		if err != nil {
			t.Fatal(err)
		}
		src := newFake()
		exp, err := f.Hosts(src, []string{"web", "vm", "gh"}, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if src.askedViaSSH != c.ViaSSH || src.askedClient != c.Client || src.inG {
			t.Errorf("%s asked viaSSH=%v client=%q git=%v", pn, src.askedViaSSH, src.askedClient, src.inG)
		}
		hosts, _, _ := src.ClientHosts([]string{"web", "vm", "gh"}, c.Client, c.ViaSSH, false)
		want, _ := c.Render(hosts)
		if len(exp.Files) != len(want.Files) {
			t.Fatalf("%s: %d files, want %d", pn, len(exp.Files), len(want.Files))
		}
		for i := range want.Files {
			if !bytes.Equal(exp.Files[i].Data, want.Files[i].Data) || exp.Files[i].Name != want.Files[i].Name {
				t.Errorf("%s: file %s differs from the client writer's", pn, want.Files[i].Name)
			}
		}
		if !strings.Contains(strings.Join(exp.Skipped, "\n"), "gh: git") {
			t.Errorf("%s: a git host was kept without IncludeGit: %v", pn, exp.Skipped)
		}
		b, err := Bundle(f, exp)
		if err != nil || len(b) == 0 {
			t.Errorf("%s: bundle: %v", pn, err)
		}
	}
}

// Termius's CSV is TermiusCSV of the hosts it can reach; a proxied host is
// left out, and IncludeGit keeps a git host.
func TestTermiusFormatIsTheTermiusCSV(t *testing.T) {
	f, _ := Get("termius")
	src := newFake()
	exp, err := f.Hosts(src, []string{"web", "vm", "gh"}, Options{IncludeGit: true})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := TermiusCSV([]Host{src.hosts["web"], src.hosts["gh"]})
	if string(exp.Files[0].Data) != want || exp.Files[0].Name != "termius-hosts.csv" {
		t.Errorf("termius = %q, want %q", exp.Files[0].Data, want)
	}
	if strings.Join(exp.Skipped, ",") != "vm: proxied" {
		t.Errorf("skipped = %v", exp.Skipped)
	}
	if !strings.Contains(strings.Join(exp.Notes, "\n"), "web  /k/web") {
		t.Errorf("no key to attach for web: %v", exp.Notes)
	}
}

// ssh_config is what the source renders for exactly the chosen hosts.
func TestSSHConfigFormatIsTheSourcesConfig(t *testing.T) {
	f, _ := Get("ssh_config")
	src := newFake()
	exp, err := f.Hosts(src, []string{"vm", "gh"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if string(exp.Files[0].Data) != "Host vm\nHost gh\n" || strings.Join(src.asked, ",") != "vm,gh" {
		t.Errorf("ssh_config = %q, asked %v", exp.Files[0].Data, src.asked)
	}
	if FileName(f, exp, "x") != "ssh_config" {
		t.Errorf("file name = %s", FileName(f, exp, "x"))
	}
}

// Remmina writes a file per host, so its single download is a zip holding
// each under its own name, even for one host.
func TestRemminaBundlesIntoAZip(t *testing.T) {
	f, _ := Get("remmina")
	for _, aliases := range [][]string{{"web"}, {"web", "gh"}} {
		exp, err := f.Hosts(newFake(), aliases, Options{IncludeGit: true})
		if err != nil {
			t.Fatal(err)
		}
		b, err := Bundle(f, exp)
		if err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(err)
		}
		if len(zr.File) != len(aliases) || zr.File[0].Name != "authama-web.remmina" {
			t.Errorf("zip holds %d files, first %q", len(zr.File), zr.File[0].Name)
		}
		if FileName(f, exp, "authama-hosts") != "authama-hosts.zip" {
			t.Errorf("file name = %s", FileName(f, exp, "authama-hosts"))
		}
	}
	if _, err := Bundle(f, &ClientExport{Files: []ClientFile{{Name: "../x", Data: []byte("x")}}}); err == nil {
		t.Error("a file name with a path went into the zip")
	}
}
