package structure

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pubDocker = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB/22QXk5vTmOo31cReZdmvraArThvJ6FLu4g9t8PFIF mac-to-devbox-docker-tunnel\n"
	pubGit    = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOxRs9PMY8J9exKFH+bcmN5kT1DXeC0ANdIPMVVVWJ4b imanimanyara/git-signing 2026-11-17\n"
)

func TestSimSixFilesClassifyAndApply(t *testing.T) {
	p := home(t, `{"version":1,"defaults":{"key_type":"ed25519","key_scope":"per_service"},"profiles":{
	  "imanimanyara":{"category":"personal","key_scope":"per_service",
	    "keys":[{"name":"git-signing","type":"ed25519"}],
	    "hosts":[{"alias":"incus","hostname":"incus","user":"imanimanyara","port":22,"key_name":"docker-tunnel",
	      "vm":{"hypervisor":"vmpal","name":"Incus Test"}}]}}}`)
	prof := filepath.Join(p.SSHDir, "profiles", "personal", "imanimanyara")
	pair(t, filepath.Join(prof, "docker-tunnel"), pubDocker)
	pair(t, filepath.Join(prof, "git-signing"), pubGit)
	pair(t, filepath.Join(p.SSHDir, "docker-tunnel"), pubDocker)
	write(t, filepath.Join(p.SSHDir, "config"), "Host incus\n  IdentityFile ~/.ssh/profiles/personal/imanimanyara/docker-tunnel\n", 0o600)
	homeDir := filepath.Dir(p.SSHDir)
	write(t, filepath.Join(homeDir, ".gitconfig"), "[user]\n\tsigningkey = ~/.ssh/profiles/personal/imanimanyara/git-signing.pub\n", 0o644)
	write(t, filepath.Join(homeDir, ".docker", "contexts", "meta", "incus", "meta.json"),
		`{"Name":"incus","Endpoints":{"docker":{"Host":"ssh://incus"}}}`, 0o644)

	ss, err := Suggest(p)
	if err != nil {
		t.Fatal(err)
	}
	got := byPath(ss)
	git := got["~/.ssh/profiles/personal/imanimanyara/git-signing"]
	if git.Kind != KindPurpose || git.Tool != "git" || git.To != "~/.ssh/profiles/personal/imanimanyara/tools/git/git-signing" || git.Plan == "" {
		t.Fatalf("git-signing: %+v", git)
	}
	if git.Confidence < 0.85 {
		t.Fatalf("git-signing confidence %v evidence %v", git.Confidence, git.Evidence)
	}
	dock := got["~/.ssh/profiles/personal/imanimanyara/docker-tunnel"]
	if dock.Kind != KindPurpose || dock.Tool != "docker" || dock.To != "~/.ssh/profiles/personal/imanimanyara/tools/docker/docker-tunnel" {
		t.Fatalf("docker-tunnel: %+v", dock)
	}
	copyS := got["~/.ssh/docker-tunnel"]
	if copyS.Kind != KindCopyRemove || copyS.Plan != "copy-remove ~/.ssh/docker-tunnel" || copyS.Info {
		t.Fatalf("leftover: %+v", copyS)
	}

	// Authama must not write gitconfig; sange is stubbed as success.
	var sangeCalls [][]string
	orig := runSange
	t.Cleanup(func() { runSange = orig })
	runSange = func(args ...string) (string, error) {
		sangeCalls = append(sangeCalls, append([]string{}, args...))
		return "updated\n", nil
	}

	moves, err := ParsePlan(git.Plan + "\n" + dock.Plan + "\n" + copyS.Plan)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	res, err := Apply(p, moves, &buf)
	if err != nil {
		t.Fatal(err, buf.String())
	}
	for _, want := range []string{
		filepath.Join(prof, "tools", "git", "git-signing"),
		filepath.Join(prof, "tools", "git", "git-signing.pub"),
		filepath.Join(prof, "tools", "docker", "docker-tunnel"),
		filepath.Join(prof, "tools", "docker", "docker-tunnel.pub"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	// originals moved away
	if _, err := os.Stat(filepath.Join(prof, "git-signing")); err == nil {
		t.Error("git-signing still at profile root")
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "docker-tunnel")); err == nil {
		t.Error("leftover docker-tunnel was not removed")
	}
	cfg, _ := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if !strings.Contains(string(cfg), "tools/docker/docker-tunnel") {
		t.Errorf("IdentityFile not rewritten: %s", cfg)
	}
	// gitconfig must be unchanged (sange owns that write)
	gc, _ := os.ReadFile(filepath.Join(homeDir, ".gitconfig"))
	if !strings.Contains(string(gc), "profiles/personal/imanimanyara/git-signing.pub") ||
		strings.Contains(string(gc), "tools/git/") {
		t.Errorf("authama must not rewrite gitconfig: %s", gc)
	}
	if len(sangeCalls) != 1 {
		t.Fatalf("sange calls: %v", sangeCalls)
	}
	joined := strings.Join(sangeCalls[0], " ")
	if !strings.Contains(joined, "profile signing retarget") || !strings.Contains(joined, "tools/git/git-signing.pub") {
		t.Errorf("handoff: %s", joined)
	}
	if !strings.Contains(joined, "--dry-run") {
		t.Errorf("preview/sandbox retarget must pass --dry-run so sange does not write ~/.gitconfig: %s", joined)
	}
	if res.Backup == "" {
		t.Error("expected a backup")
	}
}

func TestCopyRemoveRefusedWhenReferenced(t *testing.T) {
	p := home(t, `{"version":1,"defaults":{"key_type":"ed25519","key_scope":"per_service"},"profiles":{
	  "imanimanyara":{"category":"personal","key_scope":"per_service",
	    "hosts":[{"alias":"incus","hostname":"incus","user":"imanimanyara","port":22,"key_name":"docker-tunnel"}]}}}`)
	prof := filepath.Join(p.SSHDir, "profiles", "personal", "imanimanyara")
	pair(t, filepath.Join(prof, "docker-tunnel"), pubDocker)
	pair(t, filepath.Join(p.SSHDir, "docker-tunnel"), pubDocker)
	write(t, filepath.Join(p.SSHDir, "config"), "Host x\n  IdentityFile ~/.ssh/docker-tunnel\n", 0o600)
	ss, err := Suggest(p)
	if err != nil {
		t.Fatal(err)
	}
	s := byPath(ss)["~/.ssh/docker-tunnel"]
	if s.Kind != KindCopy || !s.Info {
		t.Fatalf("want info copy while referenced, got %+v", s)
	}
	_, err = ParsePlan("copy-remove ~/.ssh/docker-tunnel")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	_, err = Apply(p, []Move{{Kind: MoveCopyRemove, Src: "~/.ssh/docker-tunnel"}}, &buf)
	if err == nil || !strings.Contains(err.Error(), "still referenced") {
		t.Fatalf("want refused, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(p.SSHDir, "docker-tunnel")); err != nil {
		t.Fatal("leftover was deleted despite a reference")
	}
}
