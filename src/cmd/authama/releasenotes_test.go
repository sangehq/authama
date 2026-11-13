package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every release carries its CHANGELOG section as the body. The shared release
// workflow hands GoReleaser that section with --release-notes and reads the
// published body back, but GoReleaser reads the flag inside its changelog pipe,
// and `changelog: disable: true` skips the pipe whole: v3.2.0 and v3.4.0 were
// published with empty bodies that way, and the run reported success.
func TestReleasesCarryTheirChangelogSection(t *testing.T) {
	cfg, err := os.ReadFile(filepath.Join(filepath.Dir(repoRoot(t)), "src", ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?m)^changelog:\s*\n(?:[ \t]+#.*\n)*[ \t]+disable:\s*(true|"true")`).Match(cfg) {
		t.Error("src/.goreleaser.yaml sets changelog.disable: true, which makes GoReleaser ignore --release-notes")
	}
}

// sharedRelease is the one job release.yml may hold: a call to simtabi/.github's
// reusable Go release workflow, pinned to a commit, given the resolved tag.
var sharedRelease = regexp.MustCompile(`(?m)^[ \t]+uses:[ \t]*simtabi/\.github/\.github/workflows/go-release\.yml@[0-9a-f]{40}\b`)

// releaseTriggers are the top-level keys under `on:` in release.yml.
func releaseTriggers(t *testing.T, wf string) []string {
	t.Helper()
	m := regexp.MustCompile(`(?ms)^on:\s*\n(.*?)^\S`).FindStringSubmatch(wf)
	if m == nil {
		t.Fatal("release.yml has no on: block")
	}
	var keys []string
	for _, k := range regexp.MustCompile(`(?m)^  ([a-z_]+):`).FindAllStringSubmatch(m[1], -1) {
		keys = append(keys, k[1])
	}
	return keys
}

// A release builds on a pushed version tag, or on a dispatch naming one, and on
// nothing else (~/.claude/rules/go.md, decided 2026-11-13): no branch push,
// pull_request, schedule or release trigger, and only version tag filters. The
// work is the shared workflow's, so the pipeline is changed there once.
func TestReleaseRunsOnAVersionTagOrADispatch(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(repoRoot(t)), ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	wf := string(b)
	if got := strings.Join(releaseTriggers(t, wf), ","); got != "push,workflow_dispatch" {
		t.Errorf("release.yml triggers on %q, want exactly push,workflow_dispatch", got)
	}
	push := regexp.MustCompile(`(?ms)^  push:\s*\n(.*?)^  \S`).FindStringSubmatch(wf)
	if push == nil {
		t.Fatal("release.yml has no push trigger body")
	}
	if regexp.MustCompile(`(?m)^\s+(branches|branches-ignore|paths|tags-ignore):`).MatchString(push[1]) {
		t.Error("release.yml's push trigger filters on more than tags")
	}
	filters := regexp.MustCompile(`(?m)^\s+- '([^']+)'`).FindAllStringSubmatch(push[1], -1)
	if len(filters) == 0 {
		t.Error("release.yml's push trigger names no tag filter")
	}
	for _, f := range filters {
		if !strings.HasPrefix(f[1], "v[0-9]+.[0-9]+.[0-9]+") {
			t.Errorf("release.yml's tag filter %q is not a version", f[1])
		}
	}
	dispatch := regexp.MustCompile(`(?ms)^  workflow_dispatch:\s*\n(.*?)^\S`).FindStringSubmatch(wf)
	if dispatch == nil || !regexp.MustCompile(`(?m)^      tag:\s*\n(?:        .*\n)*?        required: true`).MatchString(dispatch[1]) {
		t.Error("release.yml's dispatch has no required tag input")
	}
	if !sharedRelease.MatchString(wf) {
		t.Error("release.yml does not call simtabi/.github's go-release.yml pinned to a commit")
	}
	if !strings.Contains(wf, "tag: ${{ inputs.tag || github.ref_name }}") {
		t.Error("release.yml does not hand the shared workflow the resolved tag")
	}
	if strings.Contains(wf, "goreleaser-action") {
		t.Error("release.yml runs GoReleaser itself; the shared workflow does")
	}
}

// bannedReleaseBlocks publish outside the GitHub release; the shared workflow
// refuses a config holding any of them at release time, and this fails first.
var bannedReleaseBlocks = regexp.MustCompile(`(?m)^(brews|homebrew_casks|scoops|winget|chocolateys|aurs|aur_sources|snapcrafts|flatpaks|nix|dockers|dockers_v2|docker_manifests|docker_signs|kos|krews|npms|cloudsmiths|furies|publishers|blobs|uploads|artifactories):`)

func TestGoReleaserPublishesOnlyToTheGitHubRelease(t *testing.T) {
	cfg, err := os.ReadFile(filepath.Join(filepath.Dir(repoRoot(t)), "src", ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	top := regexp.MustCompile(`(?m)^[a-z_]+:`).FindAll(cfg, -1)
	if len(top) < 10 {
		t.Fatalf("read %d top-level keys from src/.goreleaser.yaml; the config is not being parsed", len(top))
	}
	for _, m := range bannedReleaseBlocks.FindAll(cfg, -1) {
		t.Errorf("src/.goreleaser.yaml holds %s, which publishes outside the GitHub release", m)
	}
}
