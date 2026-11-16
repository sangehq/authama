package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every v0.1.0 binary reported "0.1.0-dirty (<40-character sha>, …)": a
// `go mod tidy` before-hook changed go.sum in the release checkout, so Go stamped
// vcs.modified=true, and the ldflags stamped the bare version and the full
// commit. These pin the three causes, and the notices file the archives carry.
func TestReleaseBuildsStampACleanVersion(t *testing.T) {
	top := repoTop(t)
	data, err := os.ReadFile(filepath.Join(top, "src", ".goreleaser.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := string(data)

	// Comments may mention the hook; only a live YAML line counts.
	live := regexp.MustCompile(`(?m)^[^#\n]*\bgo\b[^#\n]*\bmod\s+tidy\b`)
	if live.MatchString(cfg) {
		t.Error("src/.goreleaser.yaml runs `go mod tidy`; a release must build the tagged tree unmodified (make tidy-check gates tidiness)")
	}
	for _, want := range []string{
		"internal/version.Version=v{{ .Version }}",
		"internal/version.Commit={{ .ShortCommit }}",
		"- third-party-notices-go.md",
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("src/.goreleaser.yaml lacks %q", want)
		}
	}

	mk, err := os.ReadFile(filepath.Join(top, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	ci := regexp.MustCompile(`(?m)^ci:([^#\n]*)`).FindStringSubmatch(string(mk))
	if ci == nil {
		t.Fatal("Makefile has no ci: target")
	}
	for _, gate := range []string{"tidy-check", "notices-check"} {
		if !strings.Contains(" "+ci[1]+" ", " "+gate+" ") {
			t.Errorf("make ci does not run %s", gate)
		}
	}
	if _, err := os.Stat(filepath.Join(top, "third-party-notices-go.md")); err != nil {
		t.Errorf("third-party-notices-go.md is missing: %v", err)
	}
}
