package manifest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRouteFieldsAreValidated(t *testing.T) {
	ok := []string{
		`{"alias":"a","hostname":"h","user":"u","aliases":["a-cf"],"section":"prod: x (y; z)","comment":"c"}`,
		`{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"none"},"raw_options":{"ConnectTimeout":"10"}}`,
		`{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"command","command":["/usr/bin/nc","%h","%p"]}}`,
		`{"alias":"a","hostname":"h","user":"u","variants":[{"name":"warp","hostname":"10.0.0.3","proxy":{"kind":"none"}}]}`,
	}
	for _, raw := range ok {
		if _, err := hostFrom(t, raw); err != nil {
			t.Errorf("%s refused: %v", raw, err)
		}
	}
	bad := map[string]string{
		"alias with space":           `{"alias":"a","hostname":"h","user":"u","aliases":["a b"]}`,
		"alias with pattern":         `{"alias":"a","hostname":"h","user":"u","aliases":["a*"]}`,
		"alias twice":                `{"alias":"a","hostname":"h","user":"u","aliases":["a"]}`,
		"newline in comment":         `{"alias":"a","hostname":"h","user":"u","comment":"x\nHost *"}`,
		"newline in section":         `{"alias":"a","hostname":"h","user":"u","section":"x\rProxyCommand y"}`,
		"command for none":           `{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"none","command":["x"]}}`,
		"empty command":              `{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"command"}}`,
		"unknown token":              `{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"command","command":["nc","%z"]}}`,
		"newline in command":         `{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"command","command":["nc","x\ny"]}}`,
		"proxy and jump":             `{"alias":"a","hostname":"h","user":"u","proxy":{"kind":"none"},"raw_options":{"ProxyJump":"b"}}`,
		"variant name":               `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"Warp-1"}]}`,
		"variant twice":              `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"w"},{"name":"w"}]}`,
		"variant hostname":           `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"w","hostname":"x;y"}]}`,
		"variant dangerous":          `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"w","raw_options":{"LocalCommand":"x"}}]}`,
		"variant unknown field":      `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"w","user":"root"}]}`,
		"variant on a VM":            `{"alias":"a","hostname":"h","user":"u","vm":{"hypervisor":"vmpal","name":"x"},"variants":[{"name":"w"}]}`,
		"variant port":               `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"w","port":70000}]}`,
		"raw ProxyCommand stays out": `{"alias":"a","hostname":"h","user":"u","raw_options":{"ProxyCommand":"none"}}`,
	}
	for name, raw := range bad {
		if _, err := hostFrom(t, raw); err == nil {
			t.Errorf("%s: accepted %s", name, raw)
		}
	}
}

// Every name a host renders - its alias, extra aliases and variants - is one
// ssh Host pattern, so each may be declared once in the whole manifest.
func TestEveryRenderedNameIsUnique(t *testing.T) {
	for name, hosts := range map[string]string{
		"alias vs extra":   `{"alias":"a","hostname":"h","user":"u","aliases":["b"]},{"alias":"b","hostname":"h","user":"u"}`,
		"alias vs variant": `{"alias":"a","hostname":"h","user":"u","variants":[{"name":"w"}]},{"alias":"w-a","hostname":"h","user":"u"}`,
	} {
		if _, err := hostFrom(t, hosts); err == nil || !strings.Contains(err.Error(), "more than once") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRouteFieldsRoundTripAndStayOutWhenUnset(t *testing.T) {
	b, _ := json.Marshal(Host{Alias: "a", Hostname: "h", User: "u", Port: 22})
	for _, k := range []string{"aliases", "section", "comment", "variants"} {
		if strings.Contains(string(b), `"`+k+`"`) {
			t.Errorf("unset %s written: %s", k, b)
		}
	}
	m, err := hostFrom(t, `{"alias":"a","hostname":"h","user":"u","aliases":["a-cf"],"section":"s",
		"variants":[{"name":"warp","hostname":"10.0.0.3","proxy":{"kind":"none"},"raw_options":{"ConnectTimeout":10}}]}`)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(m.Profiles["p"].Hosts[0])
	want := `"aliases":["a-cf"],"section":"s","variants":[{"name":"warp","hostname":"10.0.0.3","proxy":{"kind":"none"},"raw_options":{"ConnectTimeout":10}}]`
	if !strings.Contains(string(b), want) {
		t.Errorf("got %s", b)
	}
	b, _ = json.Marshal(Variant{Name: "w"})
	if string(b) != `{"name":"w"}` {
		t.Errorf("an empty variant should be just its name: %s", b)
	}
}

func TestProxyCommandsAreReadBack(t *testing.T) {
	for in, want := range map[string]*ProxySpec{
		"none": {Kind: ProxyNone},
		"/opt/homebrew/bin/cloudflared access ssh --hostname %h":      {Kind: ProxyCloudflared},
		`"C:\Program Files\cloudflared.exe" access ssh --hostname %h`: {Kind: ProxyCloudflared},
		"exec cloudflared access ssh --hostname %h":                   {Kind: ProxyCloudflared},
		"/usr/bin/nc -X 5 -x 'proxy host:1080' %h %p":                 {Kind: ProxyCommandKind, Command: []string{"/usr/bin/nc", "-X", "5", "-x", "proxy host:1080", "%h", "%p"}},
	} {
		got, err := ProxyFromCommand(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v %v, want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"nc %h %p; touch /tmp/x", "nc $(id) %h", "nc `id`", "nc %h | tee", "nc 'x", "-oFoo"} {
		if _, err := ProxyFromCommand(in); err == nil {
			t.Errorf("%s: accepted", in)
		}
	}
	argv := []string{"/usr/bin/nc", "a b", "it's", "%h"}
	back, err := SplitCommand(ShellJoin(argv))
	if err != nil || !reflect.DeepEqual(back, argv) {
		t.Errorf("ShellJoin/SplitCommand round trip: %v %v", back, err)
	}
}

func TestHostOptionsAreCheckedAgainstOpenSSH(t *testing.T) {
	for k, v := range map[string]string{"ServerAliveInterval": "30", "connecttimeout": "10", "ProxyJump": "bastion",
		"SetEnv": `FOO="a b"`, "LocalForward": "5432 localhost:5432", "UseKeychain": "yes"} {
		if err := CheckHostOption(k, v); err != nil {
			t.Errorf("%s %s refused: %v", k, v, err)
		}
	}
	for k, v := range map[string]string{"ServerAliveIntervall": "30", "ProxyCommand": "x", "LocalCommand": "x",
		"HostName": "x", "Match": "all", "ConnectTimeout": "", "SetEnv": `FOO="a`, "Port": "22", "Compression": "#yes"} {
		if err := CheckHostOption(k, v); err == nil {
			t.Errorf("%s %q accepted", k, v)
		}
	}
	if err := CheckHostOption("ServerAliveIntervall", "1"); err == nil || !strings.Contains(err.Error(), "ServerAliveInterval") {
		t.Errorf("a typo should name the keyword: %v", err)
	}
	if CanonicalOption("connecttimeout") != "ConnectTimeout" {
		t.Error("keywords are written in OpenSSH's spelling")
	}
}
