package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// placeKey writes a stand-in private key file where the manifest expects the
// key of profile/alias, and returns its name.
func placeKey(t *testing.T, sshDir, cfg, profile, alias string) string {
	t.Helper()
	m, err := manifest.Load(filepath.Join(cfg, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range m.Profiles[profile].Hosts {
		if h.Alias != alias {
			continue
		}
		k, err := m.ResolvedKeyName(profile, h)
		if err != nil {
			t.Fatal(err)
		}
		priv := m.KeyPath(sshDir, manifest.KeyRef{Profile: profile, KeyName: k})
		if err := os.MkdirAll(filepath.Dir(priv), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(priv, []byte("not a key\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return k
	}
	t.Fatalf("no host %q in %q", alias, profile)
	return ""
}

// A load that added nothing because the agent refused every key is a failure,
// and a key file that is not there is named rather than skipped (audit B-03).
func TestLoadFailsWhenNoKeyLoadsAndNamesMissingKeys(t *testing.T) {
	p := editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "host", "add", "work", "box", "-H", "box.example.com", "-u", "deploy")
	gh := placeKey(t, p.SSHDir, p.ConfigDir, "work", "gh")
	t.Setenv("PATH", t.TempDir()) // no ssh-add: every add fails

	out, err := runErr(t, "ssh", "load", "work")
	if err == nil {
		t.Fatalf("load with every ssh-add failing exited 0:\n%s", out)
	}
	if !strings.Contains(err.Error(), gh) {
		t.Errorf("error %q does not name the key that failed (%s)", err, gh)
	}
	if !strings.Contains(out, "not on disk") || !strings.Contains(out, "box") {
		t.Errorf("the missing key file for box was not reported:\n%s", out)
	}
}
