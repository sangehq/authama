package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/apitokens"
	"github.com/sangehq/authama/src/internal/services/webhooks"
)

// CLI-level tests the e2e plan (2026-11-16) listed as missing: each of these
// commands was covered only through its service or the web UI.

// settings init writes authama.json once and refuses to replace it without
// --force.
func TestSettingsInitRefusesToReplaceWithoutForce(t *testing.T) {
	p := editHome(t)
	_ = os.Remove(p.SettingsFile())
	run(t, "settings", "init")
	if _, err := os.Stat(p.SettingsFile()); err != nil {
		t.Fatalf("settings init wrote no authama.json: %v", err)
	}
	if err := os.WriteFile(p.SettingsFile(), []byte(`{"version": 1, "display": {"timezone": "UTC"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr(t, "settings", "init"); err == nil {
		t.Error("settings init replaced an existing authama.json without --force")
	}
	if b, _ := os.ReadFile(p.SettingsFile()); !strings.Contains(string(b), `"UTC"`) {
		t.Errorf("a refused init changed authama.json:\n%s", b)
	}
	run(t, "settings", "init", "--force")
	if b, _ := os.ReadFile(p.SettingsFile()); strings.Contains(string(b), `"UTC"`) {
		t.Errorf("settings init --force kept the old authama.json:\n%s", b)
	}
}

// settings backup-key mints an age key, keeps its identity owner-only, sets
// the recipient, and never prints the secret half.
func TestSettingsBackupKeyMintsAKeyAndNeverPrintsIt(t *testing.T) {
	needBin(t, "age-keygen")
	p := editHome(t)
	out := run(t, "settings", "backup-key")
	if strings.Contains(out, "AGE-SECRET-KEY") {
		t.Fatalf("backup-key printed the secret key:\n%s", out)
	}
	b, err := os.ReadFile(p.SettingsFile())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Backup struct {
			AgeRecipient string `json:"age_recipient"`
		} `json:"backup"`
		Paths struct {
			AgeIdentityFile string `json:"age_identity_file"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(doc.Backup.AgeRecipient, "age1") {
		t.Errorf("backup.age_recipient = %q, want an age recipient", doc.Backup.AgeRecipient)
	}
	ident := doc.Paths.AgeIdentityFile
	if ident == "" {
		t.Fatal("paths.age_identity_file was not set")
	}
	fi, err := os.Stat(ident)
	if err != nil {
		t.Fatalf("the identity file is missing: %v", err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("the identity file is mode %v, want 0600", fi.Mode().Perm())
	}
	if body, _ := os.ReadFile(ident); !strings.Contains(string(body), "AGE-SECRET-KEY") {
		t.Error("the identity file holds no secret key")
	}
}

// owner forget unbinds the home: status says it is not bound afterwards.
func TestOwnerForgetUnbindsTheHome(t *testing.T) {
	ownerHome(t)
	if out := run(t, "owner", "status"); !strings.Contains(out, "bound since") {
		t.Fatalf("not bound before forget:\n%s", out)
	}
	run(t, "owner", "forget", "--yes")
	if out := run(t, "owner", "status"); !strings.Contains(out, "not bound") {
		t.Errorf("owner forget left the home bound:\n%s", out)
	}
}

// api token create shows the secret once and stores only its hash; list never
// shows it; a revoked token is gone.
func TestAPITokenCreateListRevoke(t *testing.T) {
	p := editHome(t)
	out := run(t, "api", "token", "create", "ci-reader")
	secret := ""
	for _, f := range strings.Fields(out) {
		if len(f) >= 32 && !strings.ContainsAny(f, "/:") {
			secret = f
		}
	}
	if secret == "" {
		t.Fatalf("create printed no token:\n%s", out)
	}
	if b, err := os.ReadFile(apitokens.File(p)); err != nil || strings.Contains(string(b), secret) {
		t.Fatalf("the token store holds the token in the clear (or is missing: %v)", err)
	}
	list := run(t, "api", "token", "list")
	if !strings.Contains(list, "ci-reader") || strings.Contains(list, secret) {
		t.Errorf("list should name the token and not show it:\n%s", list)
	}
	run(t, "api", "token", "revoke", "ci-reader")
	if list := run(t, "api", "token", "list"); strings.Contains(list, "ci-reader") {
		t.Errorf("a revoked token is still listed:\n%s", list)
	}
}

// webhook add refuses a non-https or private address, shows its secret once,
// keeps it owner-only and out of list, and remove takes it away.
func TestWebhookAddRefusesPrivateTargetsAndHidesTheSecret(t *testing.T) {
	p := editHome(t)
	for _, url := range []string{"http://example.com/hook", "https://127.0.0.1/hook", "https://10.0.0.5/hook"} {
		if _, err := runErr(t, "webhook", "add", "bad", url); err == nil {
			t.Errorf("webhook add took %s", url)
		}
	}
	out := run(t, "webhook", "add", "ops", "https://hooks.example.com/authama")
	if fi, err := os.Stat(webhooks.File(p)); err != nil {
		t.Fatalf("no webhook store: %v", err)
	} else if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("webhooks.json is mode %v, want 0600", fi.Mode().Perm())
	}
	eps, err := webhooks.List(p)
	if err != nil || len(eps) != 1 || eps[0].Secret == "" {
		t.Fatalf("the store holds %d endpoints (err %v); want one with a secret", len(eps), err)
	}
	list := run(t, "webhook", "list")
	if !strings.Contains(list, "ops") {
		t.Errorf("list does not name the webhook:\n%s", list)
	}
	// add shows the secret once, for the receiver; list never does.
	if strings.Count(out, eps[0].Secret) != 1 {
		t.Error("webhook add should show the signing secret exactly once")
	}
	if strings.Contains(list, eps[0].Secret) {
		t.Error("webhook list shows the signing secret")
	}
	run(t, "webhook", "disable", "ops")
	run(t, "webhook", "enable", "ops")
	run(t, "webhook", "remove", "ops")
	if list := run(t, "webhook", "list"); strings.Contains(list, "ops") {
		t.Errorf("a removed webhook is still listed:\n%s", list)
	}
}

// checklist tick and untick change only the steps a person does by hand; a
// step authama works out from disk cannot be ticked.
func TestChecklistTicksOnlyManualSteps(t *testing.T) {
	needBin(t, "ssh-keygen")
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "gh", "-H", "github.com", "-u", "git")
	run(t, "ssh", "reconcile", "--yes")
	const item = "work/work_gh-ed25519"
	state := func(step string) string {
		t.Helper()
		var d struct {
			Steps []struct{ ID, State string } `json:"steps"`
		}
		if err := json.Unmarshal([]byte(run(t, "checklist", "show", item, "--json")), &d); err != nil {
			t.Fatal(err)
		}
		for _, s := range d.Steps {
			if s.ID == step {
				return s.State
			}
		}
		t.Fatalf("no step %q in %s's checklist", step, item)
		return ""
	}
	if got := state("copies"); got == "done" {
		t.Fatalf("copies starts %q; want it open", got)
	}
	run(t, "checklist", "tick", item, "copies")
	if got := state("copies"); got != "done" {
		t.Errorf("after tick, copies is %q", got)
	}
	run(t, "checklist", "untick", item, "copies")
	if got := state("copies"); got == "done" {
		t.Errorf("after untick, copies is still done")
	}
	if _, err := runErr(t, "checklist", "tick", item, "target:gh"); err == nil {
		t.Error("a step authama works out (target:gh) was ticked by hand")
	}
}

// ssh git use rewrites the repository's origin for real, not only in a dry
// run. It proves the new URL with ls-remote first, so the test answers that
// over a stand-in ssh (GIT_SSH_COMMAND, which authama respects) serving a
// local bare repository; origin still records the real URL.
func TestGitUseRewritesTheOrigin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in ssh is a shell script")
	}
	editHome(t)
	run(t, "ssh", "profile", "add", "imanimanyara")
	run(t, "ssh", "host", "add", "imanimanyara", "github-imanimanyara", "-H", "github.com", "-u", "git")
	repo := gitRepo(t, "https://github.com/simtabi/pheg.git")
	bare := filepath.Join(t.TempDir(), "pheg.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nexec git-upload-pack '"+bare+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSH_COMMAND", fakeSSH)

	run(t, "ssh", "git", "use", "profile", repo)
	got, err := exec.Command("git", "-C", repo, "remote", "get-url", "origin").Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "git@github-imanimanyara:simtabi/pheg.git"; strings.TrimSpace(string(got)) != want {
		t.Errorf("origin = %s, want %s", strings.TrimSpace(string(got)), want)
	}
}
