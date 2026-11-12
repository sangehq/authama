package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/keystore"
	"github.com/simtabi/authama/src/internal/version"
)

// jsonOf parses a command's JSON output into a generic map.
func jsonOf(t *testing.T, out string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return got
}

// `companion info --json` is sange's handshake: the contract's names, the
// build's version and the homes this run resolves.
func TestCompanionInfoJSON(t *testing.T) {
	p := editHome(t)
	got := jsonOf(t, run(t, "companion", "info", "--json"))
	if got["name"] != "authama" || got["version"] != version.Version || got["configHome"] != p.ConfigDir || got["stateHome"] != p.State() {
		t.Errorf("companion info: %v (config %s, state %s)", got, p.ConfigDir, p.State())
	}
	if !reflect.DeepEqual(got["contractVersions"], []any{"1"}) {
		t.Errorf("contractVersions %v, want [1]", got["contractVersions"])
	}
	caps, _ := got["capabilities"].([]any)
	for _, want := range []string{"git.status.json", "git.use.json", "git.resolve.json"} {
		found := false
		for _, c := range caps {
			found = found || c == want
		}
		if !found {
			t.Errorf("capabilities %v lack %s", caps, want)
		}
	}
}

// `ssh git resolve --json` names the profile, mode, URL, alias and the key's
// fingerprint, the same one ssh-keygen reads from the public half; https has
// no alias or key. It changes nothing.
func TestGitResolveJSON(t *testing.T) {
	needBin(t, "ssh-keygen")
	p := editHome(t)
	run(t, "ssh", "profile", "add", "imanimanyara")
	run(t, "ssh", "host", "add", "imanimanyara", "github-imanimanyara", "-H", "github.com", "-u", "git")

	got := jsonOf(t, run(t, "ssh", "git", "resolve", "simtabi/authama", "--json"))
	want := map[string]any{"version": 1.0, "profile": "imanimanyara", "mode": "profile",
		"url": "git@github-imanimanyara:simtabi/authama.git", "hostAlias": "github-imanimanyara", "keyFingerprint": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("before the key is made:\n%v\nwant\n%v", got, want)
	}

	run(t, "ssh", "reconcile")
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	h, _ := manifest.GitHubHostOf(m, "imanimanyara")
	name, _ := m.ResolvedKeyName("imanimanyara", h)
	fp, err := keystore.New().Fingerprint(m.KeyPath(p.SSHDir, manifest.KeyRef{Profile: "imanimanyara", KeyName: name}) + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	got = jsonOf(t, run(t, "ssh", "git", "resolve", "simtabi/authama.git", "--json"))
	if got["keyFingerprint"] != fp || !strings.HasPrefix(fp, "SHA256:") {
		t.Errorf("keyFingerprint %v, want %s", got["keyFingerprint"], fp)
	}

	got = jsonOf(t, run(t, "ssh", "git", "resolve", "simtabi/authama", "--mode", "https", "--json"))
	if got["url"] != "https://github.com/simtabi/authama.git" || got["hostAlias"] != "" || got["keyFingerprint"] != "" {
		t.Errorf("https: %v", got)
	}
	for _, bad := range []string{"authama", "simtabi/", "/authama", "a/b/c"} {
		if _, err := runErr(t, "ssh", "git", "resolve", bad, "--json"); err == nil {
			t.Errorf("%q was accepted as owner/repo", bad)
		}
	}
	if _, err := runErr(t, "ssh", "git", "resolve", "simtabi/authama", "--profile", "nobody"); err == nil {
		t.Error("an unknown --profile was accepted")
	}
}

// `ssh git use --json --dry-run` reports each checkout and touches none.
func TestGitUseJSONDryRun(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "imanimanyara")
	run(t, "ssh", "host", "add", "imanimanyara", "github-imanimanyara", "-H", "github.com", "-u", "git")
	repo := gitRepo(t, "https://github.com/simtabi/pheg.git")
	none := t.TempDir()

	got := jsonOf(t, run(t, "ssh", "git", "use", "profile", repo, none, "--dry-run", "--json"))
	want := map[string]any{"version": 1.0, "mode": "profile", "dry_run": true, "checkouts": []any{
		map[string]any{"path": repo, "from": "https://github.com/simtabi/pheg.git", "to": "git@github-imanimanyara:simtabi/pheg.git",
			"applied": false, "skipped": "", "error": ""},
		map[string]any{"path": none, "from": "", "to": "", "applied": false,
			"skipped": "no origin (not a git checkout, or no remote named origin)", "error": ""},
	}}
	if !reflect.DeepEqual(got, want) {
		g, _ := json.MarshalIndent(got, "", "  ")
		w, _ := json.MarshalIndent(want, "", "  ")
		t.Errorf("ssh git use --json:\n%s\nwant:\n%s", g, w)
	}
	if origin, _ := os.ReadFile(repo + "/.git/config"); !strings.Contains(string(origin), "https://github.com/simtabi/pheg.git") {
		t.Error("a dry run rewrote origin")
	}
}
