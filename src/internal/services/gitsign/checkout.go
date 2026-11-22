package gitsign

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// gitTimeout bounds every git call: they are all local, so a slow one is stuck.
const gitTimeout = 30 * time.Second

// safeConfig overrides what a checkout's own config could make git run while
// authama reads it: an fsmonitor hook (git status), a pager, and the program
// that checks SSH signatures (git log --show-signature). The command line
// wins over every config file, so a checkout cannot swap them in.
var safeConfig = []string{"-c", "core.fsmonitor=false", "-c", "core.pager=cat", "-c", "gpg.ssh.program=ssh-keygen"}

// git runs git in dir with prompts off and returns its trimmed stdout. dir
// comes from a person, at the terminal or in the web UI: it is made absolute
// (so it can never read as an option) and must be a directory.
func git(dir string, args ...string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s is not a directory", dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	full := append(append(slices.Clone(safeConfig), "-C", filepath.Clean(dir)), args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(ee.Stderr)))
		}
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsCheckout reports whether dir is inside a git work tree.
func IsCheckout(dir string) bool {
	out, err := git(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// UseResult is what Use did to one checkout.
type UseResult struct {
	Dir      string
	Added    bool     // the fragment was not included before
	Replaced []string // other profiles' fragments it took out
}

// Use makes the checkout at dir include fragment from its own config, in
// place of any other fragment under fragmentDir, so it commits as that profile
// whatever directory it is in. Includes that are not authama's stay.
func Use(dir, fragment, fragmentDir string, dryRun bool) (UseResult, error) {
	res := UseResult{Dir: dir}
	if !IsCheckout(dir) {
		return res, fmt.Errorf("%s is not a git checkout", dir)
	}
	cur, _ := git(dir, "config", "--local", "--get-all", "include.path")
	var keep []string
	have := false
	for _, line := range strings.Split(cur, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case samePath(line, fragment):
			have = true
		case strings.HasSuffix(line, ".gitconfig") && samePath(filepath.Dir(line), fragmentDir):
			res.Replaced = append(res.Replaced, line)
		default:
			keep = append(keep, line)
		}
	}
	res.Added = !have
	if dryRun || have && len(res.Replaced) == 0 {
		return res, nil
	}
	if cur != "" {
		if _, err := git(dir, "config", "--local", "--unset-all", "include.path"); err != nil {
			return res, err
		}
	}
	for _, k := range append(keep, filepath.ToSlash(fragment)) {
		if _, err := git(dir, "config", "--local", "--add", "include.path", k); err != nil {
			return res, err
		}
	}
	return res, nil
}

func samePath(a, b string) bool {
	return filepath.Clean(filepath.FromSlash(a)) == filepath.Clean(filepath.FromSlash(b))
}

// CheckoutStatus is how a checkout commits: the values git resolves there,
// and the file each came from.
type CheckoutStatus struct {
	Dir        string `json:"path"`
	Checkout   bool   `json:"checkout"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	SigningKey string `json:"signing_key"`
	Format     string `json:"format"`
	Sign       bool   `json:"sign"`
	// From is the file user.email comes from, which says which fragment applies.
	From string `json:"from"`
}

// Status reads what git resolves in dir.
func Status(dir string) CheckoutStatus {
	st := CheckoutStatus{Dir: dir, Checkout: IsCheckout(dir)}
	if !st.Checkout {
		return st
	}
	get := func(key string) string { v, _ := git(dir, "config", "--get", key); return v }
	st.Email, st.Name, st.SigningKey, st.Format = get("user.email"), get("user.name"), get("user.signingkey"), get("gpg.format")
	st.Sign, _ = strconv.ParseBool(get("commit.gpgsign"))
	if out, err := git(dir, "config", "--show-origin", "--get", "user.email"); err == nil {
		if from, _, ok := strings.Cut(out, "\t"); ok {
			st.From = strings.TrimPrefix(from, "file:")
		}
	}
	return st
}

// Commit is one commit's signature, as git checks it.
type Commit struct {
	Hash    string `json:"hash"`
	Status  string `json:"status"` // git's %G?: G good, U good but unknown signer, B bad, N none, E cannot check, X/Y expired, R revoked
	Signer  string `json:"signer"`
	Subject string `json:"subject"`
}

// Good reports a signature git accepted from an allowed signer.
func (c Commit) Good() bool { return c.Status == "G" }

// Meaning says what a %G? letter means.
func Meaning(status string) string {
	switch status {
	case "G":
		return "good signature from an allowed signer"
	case "U":
		return "good signature, signer not in allowed_signers"
	case "B":
		return "BAD signature"
	case "X":
		return "good signature that has expired"
	case "Y":
		return "good signature by an expired key"
	case "R":
		return "good signature by a revoked key"
	case "E":
		return "cannot check (no allowed_signers file, or ssh-keygen missing)"
	case "N":
		return "not signed"
	}
	return "unknown (" + status + ")"
}

// Verify checks the signatures of the last n commits in dir against
// allowedSigners (the checkout's own setting when empty).
func Verify(dir, allowedSigners string, n int) ([]Commit, error) {
	if !IsCheckout(dir) {
		return nil, fmt.Errorf("%s is not a git checkout", dir)
	}
	if n <= 0 {
		n = 10
	}
	args := []string{}
	if allowedSigners != "" {
		args = append(args, "-c", "gpg.ssh.allowedSignersFile="+filepath.ToSlash(allowedSigners))
	}
	args = append(args, "log", "-n", strconv.Itoa(n), "--format=%H%x1f%G?%x1f%GS%x1f%s")
	out, err := git(dir, args...)
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) != 4 {
			continue
		}
		commits = append(commits, Commit{Hash: f[0], Status: f[1], Signer: f[2], Subject: f[3]})
	}
	return commits, nil
}
