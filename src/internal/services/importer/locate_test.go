package importer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/paths"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func hostAliases(t *testing.T, src Source) []string {
	t.Helper()
	var out []string
	for _, h := range parseSources(src) {
		out = append(out, h.Alias)
	}
	sort.Strings(out)
	return out
}

// Every layout an ssh setup comes in is read: a file, a folder whose config
// Includes the rest (flat or nested), an XDG-style config home, and a folder
// of config files with no top-level config at all.
func TestLocateReadsEveryLayout(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		at    string // relative to the root; "" is the root folder
		want  string
		nFile int
	}{
		{"a config file", map[string]string{"config": "Host a\n  HostName a.example\n"}, "config", "a", 1},
		{"a folder with config and config.d", map[string]string{
			"config":             "Include config.d/*\nHost a\n  HostName a.example\n",
			"config.d/10-b.conf": "Host b\n  HostName b.example\n",
			"config.d/20-c.conf": "Host c\n  HostName c.example\n",
		}, "", "a b c", 1},
		{"nested profiles Included", map[string]string{
			"config":                       "Include profiles/*/config profiles/*/*/config\n",
			"profiles/work/config":         "Host w\n  HostName w.example\n",
			"profiles/clients/acme/config": "Host acme\n  HostName acme.example\n",
		}, "", "acme w", 1},
		{"a config home with ssh/config", map[string]string{
			"ssh/config":        "Include conf.d/*.conf\n",
			"ssh/conf.d/x.conf": "Host x\n  HostName x.example\n",
		}, "", "x", 1},
		{"no top-level config, nested", map[string]string{
			"profiles/work/config":         "Host w\n  HostName w.example\n",
			"profiles/clients/acme/config": "Host acme\n  HostName acme.example\n",
			"profiles/work/known_hosts":    "w.example ssh-ed25519 AAAA\n",
		}, "", "acme w", 2},
		{"a flat folder of .conf files", map[string]string{
			"one.conf": "Host one\n  HostName one.example\n",
			"two.conf": "Host two\n  HostName two.example\n",
		}, "", "one two", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, tc.files)
			src, err := Locate(filepath.Join(root, tc.at))
			if err != nil {
				t.Fatal(err)
			}
			if len(src.Files) != tc.nFile {
				t.Errorf("read %d files %v, want %d", len(src.Files), src.Files, tc.nFile)
			}
			if got := strings.Join(hostAliases(t, src), " "); got != tc.want {
				t.Errorf("hosts %q, want %q", got, tc.want)
			}
		})
	}
}

// A folder with nothing ssh can read is an error that says where it looked.
func TestLocateRefusesAFolderWithNoConfig(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"id_ed25519.pub": "ssh-ed25519 AAAA\n", "known_hosts": "x\n"})
	_, err := Locate(root)
	if err == nil || !strings.Contains(err.Error(), "looked for config") {
		t.Fatalf("err = %v", err)
	}
	if _, err := Locate(filepath.Join(root, "missing")); err == nil {
		t.Fatal("a missing path was accepted")
	}
}

// Importing another machine's ~/.ssh, an IdentityFile under ~/.ssh/ names a
// key in that copy, not in this machine's ~/.ssh.
func TestImportingAnotherFolderFindsItsKeys(t *testing.T) {
	old := t.TempDir()
	writeFiles(t, old, map[string]string{
		"config": "Host box\n  HostName box.example\n  User me\n  IdentityFile ~/.ssh/old_key\n",
	})
	mintKey(t, filepath.Join(old, "old_key"))
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	res, err := New(p, false).Run(old, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.KeysFound != 1 {
		t.Errorf("keys found %d, want 1: the copy's key was not looked for in the copy", res.KeysFound)
	}
	if res.Root != old || len(res.Sources) != 1 {
		t.Errorf("root %q sources %v", res.Root, res.Sources)
	}
}
