package gitsign

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// MinGit is the oldest git that signs and verifies with SSH keys
// (gpg.format=ssh, gpg.ssh.allowedSignersFile). It also reads settings from
// GIT_CONFIG_COUNT, which arrived in 2.31.
const MinGit = "2.34"

var gitVersionRe = regexp.MustCompile(`git version (\d+)\.(\d+)`)

// parseGitVersion is the major and minor of `git version` output.
func parseGitVersion(out string) (int, int, bool) {
	m := gitVersionRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return major, minor, true
}

// CheckGit says when git is missing or older than MinGit, in words a person
// can act on; nil when it is new enough.
func CheckGit() error {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "version").Output()
	if err != nil {
		return fmt.Errorf("git is not installed or did not run: SSH signing needs git %s or newer", MinGit)
	}
	return checkGitVersion(strings.TrimSpace(string(out)))
}

func checkGitVersion(out string) error {
	major, minor, ok := parseGitVersion(out)
	if !ok {
		return nil // an unusual build: let git itself answer
	}
	if major > 2 || major == 2 && minor >= 34 {
		return nil
	}
	return fmt.Errorf("%s is too old for SSH signing: it needs git %s or newer", out, MinGit)
}
