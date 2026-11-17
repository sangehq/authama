package renderer

import (
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

func adoptManifest(t *testing.T) (*manifest.Manifest, manifest.Host) {
	t.Helper()
	kn := "imported_web-ed25519"
	h := manifest.Host{Alias: "web", Hostname: "web.example", User: "deploy", Port: 2222, KeyName: &kn,
		RawOptions: manifest.NewOrderedOptions([][2]string{{"serveraliveinterval", "15"}})}
	m := manifest.Starter(false)
	m.SetProfile("imported", manifest.Profile{KeyScope: "per_service", Hosts: []manifest.Host{h}})
	return m, h
}

// Represents is the rule both import and config adopt use before removing a
// hand-written line: true only when the rendered block reproduces it.
func TestRepresentsOnlyWhatTheRenderReproduces(t *testing.T) {
	m, h := adoptManifest(t)
	cases := []struct {
		key, val string
		want     bool
	}{
		{"HostName", "web.example", true},
		{"hostname", `"WEB.example"`, true},
		{"HostName", "other.example", false},
		{"User", "deploy", true},
		{"User", "root", false},
		{"Port", "2222", true},
		{"Port", "22", false},
		{"IdentityFile", "~/.ssh/profiles/imported/imported_web-ed25519", true},
		{"IdentityFile", "~/.ssh/id_other", false},
		{"ServerAliveInterval", "15", true},
		{"ServerAliveInterval", "30", false},
		{"IdentitiesOnly", "yes", true},
		{"UserKnownHostsFile", "~/.ssh/known_hosts", true},
		{"UserKnownHostsFile", "~/.ssh/known_hosts_web", false},
		{"ProxyCommand", "ssh -W %h:%p jump", false},
		{"ForwardAgent", "yes", false},
	}
	for _, tc := range cases {
		got, why := Represents(m, "imported", h, OptionLine{Key: tc.key, Value: tc.val}, nil)
		if got != tc.want {
			t.Errorf("%s %s: got %v (%s), want %v", tc.key, tc.val, got, why, tc.want)
		}
		if !got && why == "" {
			t.Errorf("%s %s: refused without a reason", tc.key, tc.val)
		}
	}
	// An adopted copy of the key counts as the same key.
	same := func(string) bool { return true }
	if ok, _ := Represents(m, "imported", h, OptionLine{Key: "IdentityFile", Value: "/elsewhere/key"}, same); !ok {
		t.Error("sameKey was not consulted for IdentityFile")
	}
}

func TestStripLinesRemovesRangesAndPlacesTheMarkers(t *testing.T) {
	in := "# top\nHost a\n    User x\n\nHost b\n    User y\n\nHost *\n    User z\n"
	out := StripLines(in, []LineRange{{Start: 1, End: 3}, {Start: 4, End: 6}}, 1)
	want := "# top\n" + ManagedHeader + "\n" + ManagedEnd + "\n\nHost *\n    User z\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
	// With a managed block already there, the markers are not added again.
	twice := StripLines(ComposeRootConfig(out, ManagedHeader+"\n"+ManagedEnd+"\n"), nil, 0)
	if strings.Count(twice, ManagedHeader) != 1 {
		t.Errorf("markers added to a config that has them:\n%s", twice)
	}
	// Nothing to drop and nowhere to put markers: unchanged.
	if got := StripLines(in, nil, -1); got != in {
		t.Errorf("a no-op strip changed the content:\n%q", got)
	}
}
