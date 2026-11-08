package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
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

// golangci-lint-action downloads its binary from the same CDN, so ci.yml runs
// it as two attempts: the first may fail, and a second, identical one runs
// only if it did. The guard is the pairing: an attempt allowed to fail with no
// second attempt behind it would let a real lint finding pass the job.
func TestTheLintActionGetsASecondAttempt(t *testing.T) {
	type step struct {
		ID              string            `yaml:"id"`
		If              string            `yaml:"if"`
		Uses            string            `yaml:"uses"`
		ContinueOnError any               `yaml:"continue-on-error"`
		With            map[string]string `yaml:"with"`
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []step `yaml:"steps"`
		} `yaml:"jobs"`
	}
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "..", ".github", "workflows", "ci.yml")) //nolint:gosec // the repository's own workflow
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, &wf); err != nil {
		t.Fatal(err)
	}
	pairs := 0
	for name, job := range wf.Jobs {
		for i, s := range job.Steps {
			if !strings.HasPrefix(s.Uses, "golangci/golangci-lint-action@") || s.If != "" {
				continue
			}
			if s.ContinueOnError != true || s.ID == "" {
				t.Errorf("%s: the lint attempt at step %d has no id or is not continue-on-error, so a CDN 504 fails the job", name, i)
				continue
			}
			if i+1 >= len(job.Steps) {
				t.Errorf("%s: the lint attempt %q has no second attempt after it", name, s.ID)
				continue
			}
			next := job.Steps[i+1]
			want := "steps." + s.ID + ".outcome == 'failure'"
			if next.Uses != s.Uses || next.If != want || next.ContinueOnError != nil ||
				next.With["version"] != s.With["version"] || next.With["working-directory"] != s.With["working-directory"] {
				t.Errorf("%s: the step after %q is not an identical second attempt under if: %s (got %+v)", name, s.ID, want, next)
				continue
			}
			pairs++
		}
	}
	if pairs == 0 {
		t.Fatal("found no lint attempt in ci.yml; the scan is not matching it")
	}
}

// The PowerShell installer gets the same: every request it makes to GitHub,
// the version lookup's two and the downloads, runs through Invoke-Retried.
func TestEveryWindowsInstallerRequestRetries(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "scripts", "install.ps1")) //nolint:gosec // the repository's own installer
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	calls := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.Contains(line, "Invoke-RestMethod") || strings.Contains(line, "Invoke-WebRequest"):
			calls++
			if !strings.Contains(line, "Invoke-Retried {") {
				t.Errorf("install.ps1:%d: a request outside Invoke-Retried:\n  %s", i+1, line)
			}
		case strings.Contains(line, ".GetResponse()"):
			calls++
			inside := false
			for j := i - 1; j >= 0 && j >= i-5; j-- {
				inside = inside || strings.Contains(lines[j], "Invoke-Retried {")
			}
			if !inside {
				t.Errorf("install.ps1:%d: the releases/latest request is not inside an Invoke-Retried block", i+1)
			}
		}
	}
	if calls < 3 {
		t.Fatalf("found %d requests in install.ps1, want at least 3 (API lookup, redirect fallback, download); the scan is not matching them", calls)
	}
}

// CI's installer step installs from a local release, and names the version, so
// a github.com outage cannot fail it: on 2026-11-08 the version lookup met 504s
// for longer than its retries last, on both test legs. The lookup itself is
// tested in the same step against file:// stand-ins.
func TestCIInstallsNameTheirVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "..", ".github", "workflows", "ci.yml")) //nolint:gosec // the repository's own workflow
	if err != nil {
		t.Fatal(err)
	}
	installs := regexp.MustCompile(`bash src/scripts/install\.sh(\s+(\S+))?`).FindAllStringSubmatch(string(b), -1)
	if len(installs) < 2 {
		t.Fatalf("found %d runs of install.sh in ci.yml, want at least 2; the scan is not matching them", len(installs))
	}
	for _, m := range installs {
		if !strings.HasPrefix(m[2], "v") {
			t.Errorf("ci.yml runs install.sh without a version, so it resolves one from github.com: %q", m[0])
		}
	}
	if !strings.Contains(string(b), `lookup "file://`) {
		t.Error("ci.yml no longer tests the version lookup against its stand-ins")
	}
}
