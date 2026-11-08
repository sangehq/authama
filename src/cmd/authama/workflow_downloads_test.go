package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A download in CI must survive the transient 5xx GitHub's release CDN
// answers now and then: on 2026-11-08 one 504 failed the gitleaks job on
// authama#160 before it scanned anything. So every curl a workflow runs
// retries, connection errors included.
func TestEveryWorkflowDownloadRetries(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "..", ".github", "workflows")
	files, err := filepath.Glob(filepath.Join(dir, "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Fatalf("read %d workflow files in %s; the scan is not reading them", len(files), dir)
	}
	// A curl command: the word followed by a flag, wherever it starts (a run
	// line, after a pipe, inside an if), with its continuation lines joined.
	// Comments are prose, not commands.
	curl := regexp.MustCompile(`(^|[\s|;&(])curl\s+-`)
	calls := 0
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // the repository's own workflows
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i := 0; i < len(lines); i++ {
			line := strings.TrimSpace(lines[i])
			if strings.HasPrefix(line, "#") || !curl.MatchString(line) {
				continue
			}
			cmd := line
			for strings.HasSuffix(cmd, `\`) && i+1 < len(lines) {
				i++
				cmd = strings.TrimSuffix(cmd, `\`) + " " + strings.TrimSpace(lines[i])
			}
			calls++
			for _, want := range []string{"--retry ", "--retry-all-errors"} {
				if !strings.Contains(cmd, want) {
					t.Errorf("%s: a download without %s:\n  %s", filepath.Base(f), strings.TrimSpace(want), cmd)
				}
			}
		}
	}
	if calls == 0 {
		t.Fatal("found no curl in any workflow; the scan is not matching them")
	}
}

// The same holds for the shell installer, run by users and by CI's installer
// job: every request it makes retries a transient answer, through the one
// CURL_RETRY array it defines.
func TestEveryInstallerDownloadRetries(t *testing.T) {
	path := filepath.Join(repoRoot(t), "scripts", "install.sh")
	b, err := os.ReadFile(path) //nolint:gosec // the repository's own installer
	if err != nil {
		t.Fatal(err)
	}
	curl := regexp.MustCompile(`(^|[\s|;&($])curl\s+-`)
	calls := 0
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !curl.MatchString(line) {
			continue
		}
		calls++
		if !strings.Contains(line, `"${CURL_RETRY[@]}"`) {
			t.Errorf("install.sh:%d: a request without \"${CURL_RETRY[@]}\":\n  %s", i+1, line)
		}
	}
	if calls < 4 {
		t.Fatalf("found %d curl requests in install.sh, want at least 4 (version lookup, its fallback, checksums, binary); the scan is not matching them", calls)
	}
	if !strings.Contains(string(b), "CURL_RETRY=(--retry ") {
		t.Error("install.sh no longer defines CURL_RETRY with --retry")
	}
}
