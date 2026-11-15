package checklist

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

// work/work_gh is in use: deployed and verified on gh-work, pasted by hand on
// panel. work/work_spare is declared with no host. personal/personal_gh was
// revoked, and removing it from gh-personal failed.
const manifestJSON = `{
  "version": 1,
  "defaults": {"key_type": "ed25519", "rotate_after_days": 365},
  "profiles": {
    "work": {"key_scope": "per_service",
      "keys": [{"name": "work_spare-ed25519"}],
      "hosts": [{"alias": "gh-work", "hostname": "github.com", "user": "git", "key_name": "work_gh-ed25519"},
                {"alias": "panel", "hostname": "panel.example", "user": "ops", "key_name": "work_gh-ed25519"}]},
    "personal": {"key_scope": "per_service",
      "hosts": [{"alias": "gh-personal", "hostname": "github.com", "user": "git", "key_name": "personal_gh-ed25519"}]}
  }
}`

const inventoryJSON = `{
  "version": 1,
  "keys": {
    "SHA256:work": {"profile": "work", "path": "~/.ssh/profiles/work/work_gh-ed25519",
      "type": "ed25519", "comment": null, "created": "2026-01-01", "rotate_after_days": 365,
      "expires_on": "2027-01-01", "deployments": [
        {"target": "gh-work", "method": "ssh-copy-id", "date": "2026-01-02", "verified": true},
        {"target": "panel", "method": "web-panel", "date": null, "verified": false}]},
    "SHA256:personal": {"profile": "personal", "path": "~/.ssh/profiles/personal/personal_gh-ed25519",
      "type": "ed25519", "comment": null, "created": "2025-01-01", "rotate_after_days": 365,
      "expires_on": "2026-01-01", "deployments": [{"target": "gh-personal", "method": "ssh-copy-id", "date": null, "verified": true}]},
    "SHA256:retired": {"profile": "work", "path": "~/.ssh/profiles/work/old/work_gh-ed25519",
      "type": "ed25519", "comment": null, "created": "2025-01-01", "rotate_after_days": 365,
      "expires_on": "2026-01-01", "deployments": []}
  }
}`

type home struct {
	p   paths.Paths
	svc *Service
}

func fixture(t *testing.T) home {
	t.Helper()
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg"), DevRoot: base}
	mustMkdir(t, p.ConfigDir)
	write(t, p.Manifest(), manifestJSON, 0o600)
	write(t, p.Inventory(), inventoryJSON, 0o600)
	for _, k := range []struct{ profile, name string }{
		{"work", "work_gh-ed25519"}, {"work", "work_spare-ed25519"}, {"personal", "personal_gh-ed25519"},
	} {
		dir := filepath.Join(p.SSHDir, "profiles", k.profile)
		mustMkdir(t, dir)
		write(t, filepath.Join(dir, k.name), "PRIVATE\n", 0o600)
		write(t, filepath.Join(dir, k.name+".pub"), "ssh-ed25519 AAAA "+k.name+"\n", 0o644)
	}
	oldDir := filepath.Join(p.SSHDir, "profiles", "work", "old")
	mustMkdir(t, oldDir)
	write(t, filepath.Join(oldDir, "work_gh-ed25519"), "OLD PRIVATE\n", 0o600)

	revs := inventory.Revocations{"SHA256:personal": {Key: "personal/personal_gh-ed25519", RevokedOn: "2026-11-01",
		Removed: []string{}, NotRemoved: []string{"gh-personal"}}}
	if err := revs.Save(inventory.RevokedPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
	rets := inventory.Retirements{"SHA256:retired": {Key: "work/work_gh-ed25519", Successor: "SHA256:work", RetiredOn: "2026-10-01",
		Removed: []string{"gh-work"}, NotRemoved: []string{"panel"}, ByHand: map[string]string{"panel": "https://panel.example/keys"}}}
	if err := rets.Save(inventory.RetiredPath(p.Inventory())); err != nil {
		t.Fatal(err)
	}
	// An npm token installed in the (sandbox) home's .npmrc.
	write(t, filepath.Join(base, paths.DevHomeDir, ".npmrc"),
		"//registry.npmjs.org/:_authToken="+testcred.Token("npm_", "npm")+"\n", 0o600)

	svc := New(p)
	svc.Now = func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	svc.Getenv = func(string) string { return "" }
	// The agent holds the revoked key: it can still be offered.
	svc.AgentFingerprints = func() (map[string]bool, bool) { return map[string]bool{"SHA256:personal": true}, true }
	return home{p: p, svc: svc}
}

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func step(t *testing.T, c Checklist, id string) Step {
	t.Helper()
	for _, s := range c.Steps {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("%s has no step %q; steps: %v", c.Name, id, ids(c))
	return Step{}
}

func ids(c Checklist) []string {
	var out []string
	for _, s := range c.Steps {
		out = append(out, s.ID)
	}
	return out
}

func TestEveryRevocableItemHasAChecklist(t *testing.T) {
	h := fixture(t)
	all, err := h.svc.All()
	if err != nil {
		t.Fatal(err)
	}
	// A live key and its retired predecessor share a name, so modes are kept
	// per name.
	got := map[string]string{}
	for _, c := range all {
		k := c.Name + "@" + c.Kind
		got[k] = strings.TrimPrefix(got[k]+","+c.Mode, ",")
	}
	want := map[string]string{
		"work/work_gh-ed25519@key":         InUse + "," + Retired,
		"work/work_spare-ed25519@key":      InUse,
		"personal/personal_gh-ed25519@key": Revoked,
		"npm registry.npmjs.org@token":     InUse,
	}
	for k, mode := range want {
		if got[k] != mode {
			t.Errorf("%s: mode %q, want %q (all: %v)", k, got[k], mode, got)
		}
	}
	retired, err := h.svc.Get("key:SHA256:retired")
	if err != nil || retired.Mode != Retired {
		t.Fatalf("the rotated-out key: %+v, %v", retired.Mode, err)
	}
}

func TestAKeyInUseListsWhereItIs(t *testing.T) {
	h := fixture(t)
	c, err := h.svc.Get("work/work_gh-ed25519")
	if err != nil {
		t.Fatal(err)
	}
	// Off Windows the fixture's 0600 key is owner-only. On Windows a temp
	// file inherits its directory's ACL, so only the step's presence is
	// checked there.
	if s := step(t, c, "files"); runtime.GOOS != "windows" && s.State != Done {
		t.Errorf("files: %+v", s)
	}
	if s := step(t, c, "target:gh-work"); s.State != Done || !strings.Contains(s.Title, "verified") {
		t.Errorf("a verified deployment: %+v", s)
	}
	// A web-panel target is one authama cannot see: it is ticked by hand.
	if s := step(t, c, "target:panel"); !s.Manual || s.State != Todo {
		t.Errorf("a by-hand target: %+v", s)
	}
	if s := step(t, c, "expiry"); s.State != Done {
		t.Errorf("not yet due: %+v", s)
	}
}

func TestARevokedKeyListsWhatIsLeft(t *testing.T) {
	h := fixture(t)
	c, err := h.svc.Get("personal/personal_gh-ed25519")
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != Revoked || c.Since != "2026-11-01" {
		t.Fatalf("mode %q since %q", c.Mode, c.Since)
	}
	if s := step(t, c, "removed:gh-personal"); s.State != Todo || !strings.Contains(s.Fix, "ssh key revoke personal/personal_gh-ed25519") {
		t.Errorf("the target it could not be removed from: %+v", s)
	}
	if s := step(t, c, "hosts"); s.State != Todo || !strings.Contains(s.Fix, "ssh keygen gh-personal --force") {
		t.Errorf("a host still wired to it: %+v", s)
	}
	if s := step(t, c, "files"); s.State != Todo || !strings.Contains(s.Fix, "--purge") {
		t.Errorf("the private key still on disk: %+v", s)
	}
	if s := step(t, c, "agent"); s.State != Todo || !strings.HasPrefix(s.Fix, "ssh-add -d ") {
		t.Errorf("still in the agent: %+v", s)
	}
	if c.Complete() {
		t.Fatal("a revocation with work left reads as complete")
	}
}

func TestARetiredKeyKeepsWhatTheRotationDid(t *testing.T) {
	h := fixture(t)
	c, err := h.svc.Get("key:SHA256:retired")
	if err != nil {
		t.Fatal(err)
	}
	if s := step(t, c, "removed:gh-work"); s.State != Done {
		t.Errorf("removed by the rotation: %+v", s)
	}
	if s := step(t, c, "removed:panel"); !s.Manual || s.Link != "https://panel.example/keys" {
		t.Errorf("a by-hand target says where: %+v", s)
	}
	if s := step(t, c, "files"); s.State != Todo || s.Fix != "" || !strings.Contains(s.Detail, "rollback") {
		t.Errorf("the old/ copy is kept for rollback, not purged by a command: %+v", s)
	}
}

func TestTicksPersistAndOnlyManualStepsTick(t *testing.T) {
	h := fixture(t)
	c, err := h.svc.Tick("work/work_gh-ed25519", "target:panel", true)
	if err != nil {
		t.Fatal(err)
	}
	if s := step(t, c, "target:panel"); s.State != Done || s.Ticked == "" {
		t.Fatalf("ticked: %+v", s)
	}
	// A new service reads the tick back from checklists.json.
	again, _ := New(h.p).Get(c.ID)
	if s := step(t, again, "target:panel"); s.State != Done {
		t.Fatalf("the tick was not kept: %+v", s)
	}
	if fi, err := os.Stat(StorePath(h.p)); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("checklists.json: %v %v", fi, err)
	}
	if _, err := h.svc.Tick(c.ID, "files", true); err == nil {
		t.Fatal("a step authama checks itself was ticked by hand")
	}
	if _, err := h.svc.Tick(c.ID, "nope", true); err == nil {
		t.Fatal("an unknown step was ticked")
	}
	c, _ = h.svc.Tick(c.ID, "target:panel", false)
	if s := step(t, c, "target:panel"); s.State != Todo {
		t.Fatalf("untick: %+v", s)
	}
}

func TestATokensRevocationStartsWhenAsked(t *testing.T) {
	h := fixture(t)
	id := "token:npm/registry.npmjs.org"
	c, err := h.svc.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != InUse || !strings.HasSuffix(step(t, c, "file:"+fileOf(t, c)).Title, ".npmrc") {
		t.Fatalf("in use: %+v", c)
	}
	c, err = h.svc.SetRevoking(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if c.Mode != Revoking || c.Since != "2026-11-08" {
		t.Fatalf("revoking: %q %q", c.Mode, c.Since)
	}
	if s := step(t, c, "issuer"); !s.Manual || s.Link != "https://www.npmjs.com/settings/~/tokens" {
		t.Errorf("revoke at the issuer: %+v", s)
	}
	if s := step(t, c, "file:"+fileOf(t, c)); s.State != Todo || s.Fix != "authama token delete npm registry.npmjs.org" {
		t.Errorf("remove it from the file: %+v", s)
	}
	if _, err := h.svc.SetRevoking("work/work_gh-ed25519", true); err == nil {
		t.Fatal("a key's checklist was switched by hand")
	}
	c, _ = h.svc.SetRevoking(id, false)
	if c.Mode != InUse {
		t.Fatalf("stopped: %q", c.Mode)
	}
}

func fileOf(t *testing.T, c Checklist) string {
	t.Helper()
	for _, s := range c.Steps {
		if f, ok := strings.CutPrefix(s.ID, "file:"); ok {
			return f
		}
	}
	t.Fatalf("no file step in %v", ids(c))
	return ""
}

func TestUnknownItem(t *testing.T) {
	h := fixture(t)
	if _, err := h.svc.Get("key:SHA256:none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestNoAgentIsUnknownNotDone(t *testing.T) {
	h := fixture(t)
	h.svc.AgentFingerprints = func() (map[string]bool, bool) { return nil, false }
	c, _ := h.svc.Get("personal/personal_gh-ed25519")
	if s := step(t, c, "agent"); s.State != Unknown {
		t.Fatalf("no agent: %+v", s)
	}
	if c.Complete() {
		t.Fatal("an unknown step counts as done")
	}
}
