// Package gitsign renders each profile's git identity and SSH commit signing
// from the manifest (profiles.<name>.git), and applies it per directory and per
// checkout.
//
// What it writes, all from the manifest and all re-rendered on every apply:
//
//   - <config home>/git/<profile>.gitconfig: the profile's user.email, user.name
//     and user.signingkey, and, when it signs, gpg.format=ssh, the
//     allowed_signers file, commit.gpgsign and tag.gpgsign;
//   - <config home>/git/allowed_signers: one line per signing profile, so `git
//     log --show-signature` and `git verify-commit` can tell a good signature;
//   - one managed block in the global git config with an includeIf per
//     directory a profile claims, pointing at its fragment.
//
// The block is the only thing written outside authama's own homes. It sits
// between markers and is the only part of that file authama ever changes; the
// file is copied to the state home before each change, and a symlinked
// ~/.gitconfig (a dotfiles checkout) is written through, so the link stays.
// No private key is read: signing names the public half, and ssh-keygen (or
// the 1Password agent) finds the private one when git signs.
package gitsign

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// Markers delimit the block in the global git config.
const (
	BeginMarker = "# >>> authama: git identities (managed by `authama ssh git sign apply`; edit the manifest, not this block) >>>"
	EndMarker   = "# <<< authama: git identities <<<"
)

// fragmentHeader opens every fragment, so a reader of the file knows where it
// comes from and that an edit there does not last.
const fragmentHeader = "# Written by `authama ssh git sign apply` from the manifest (profiles.%s.git).\n" +
	"# It is rewritten on every apply: change the manifest, not this file.\n"

// Dir is where the fragments and allowed_signers live.
func Dir(p paths.Paths) string { return filepath.Join(p.ConfigDir, "git") }

// FragmentPath is a profile's git config fragment.
func FragmentPath(p paths.Paths, profile string) string {
	return filepath.Join(Dir(p), profile+".gitconfig")
}

// AllowedSignersPath is the allowed_signers file every fragment names.
func AllowedSignersPath(p paths.Paths) string { return filepath.Join(Dir(p), "allowed_signers") }

// GlobalConfigPath is the global git config the includeIf block goes in: in a
// sandbox, the sandbox home's .gitconfig; else $GIT_CONFIG_GLOBAL, else
// ~/.gitconfig, or $XDG_CONFIG_HOME/git/config when only that one exists.
func GlobalConfigPath(p paths.Paths, getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	if p.IsDev() {
		return filepath.Join(p.TokenHome(paths.Getenv(getenv)), ".gitconfig")
	}
	if g := getenv("GIT_CONFIG_GLOBAL"); g != "" {
		return g
	}
	home := p.TokenHome(paths.Getenv(getenv))
	dot := filepath.Join(home, ".gitconfig")
	if _, err := os.Stat(dot); err != nil {
		xdg := getenv("XDG_CONFIG_HOME")
		if xdg == "" {
			xdg = filepath.Join(home, ".config")
		}
		if alt := filepath.Join(xdg, "git", "config"); fs.Exists(alt) {
			return alt
		}
	}
	return dot
}

// Signer is one profile's resolved git identity.
type Signer struct {
	Profile string
	Git     manifest.ProfileGit
	Key     manifest.KeyRef
	// PubPath is the public half that signs; PubLine its "type base64" with the
	// comment dropped. PubLine is empty when the key is not made yet.
	PubPath     string
	PubLine     string
	OnePassword bool
}

// Signers resolves every profile with a git block, in name order. A profile
// whose key cannot be found is reported in problems and left out, so the
// others still apply.
func Signers(m *manifest.Manifest, sshDir string) (out []Signer, problems []string) {
	for _, name := range m.ProfileNames() {
		prof := m.Profiles[name]
		if prof.Git == nil {
			continue
		}
		s := Signer{Profile: name, Git: *prof.Git, OnePassword: prof.InOnePassword()}
		key := prof.Git.SigningKey
		if key == "" {
			h, ok := manifest.GitHubHostOf(m, name)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: no git.signing_key and no github.com host to take a key from", name))
				continue
			}
			k, err := m.ResolvedKeyName(name, h)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			key = k
		}
		s.Key = manifest.KeyRef{Profile: name, KeyName: key}
		if !declares(m, s.Key) {
			problems = append(problems, fmt.Sprintf("%s: git.signing_key %q is not one of the profile's keys", name, key))
			continue
		}
		s.PubPath = m.KeyPath(sshDir, s.Key) + ".pub"
		if b, err := os.ReadFile(s.PubPath); err == nil {
			if f := strings.Fields(string(b)); len(f) >= 2 {
				s.PubLine = f[0] + " " + f[1]
			}
		}
		if s.PubLine == "" && prof.Git.Sign {
			problems = append(problems, fmt.Sprintf("%s: %s has no public key yet (run `authama ssh reconcile`); signing is left off until it has", name, s.Key))
		}
		out = append(out, s)
	}
	return out, problems
}

func declares(m *manifest.Manifest, ref manifest.KeyRef) bool {
	refs, err := m.KeyRefs()
	if err != nil {
		return false
	}
	for _, r := range refs {
		if r == ref {
			return true
		}
	}
	return false
}

// signs reports whether s's fragment turns signing on: asked for, and a key to sign with.
func (s Signer) signs() bool { return s.Git.Sign && s.PubLine != "" }

// Write is one file an apply writes, with what is there now ("" when absent).
type Write struct {
	Path, Old, New string
	Mode           os.FileMode
}

// Changed reports whether the write changes anything.
func (w Write) Changed() bool { return w.Old != w.New }

// Plan is everything an apply would do.
type Plan struct {
	Signers  []Signer
	Problems []string
	Writes   []Write
	// Removes are fragments of profiles that no longer have a git block.
	Removes []string
	// Global is the global git config the block is in.
	Global string
}

// Pending is the writes that change something.
func (pl Plan) Pending() []Write {
	var out []Write
	for _, w := range pl.Writes {
		if w.Changed() {
			out = append(out, w)
		}
	}
	return out
}

// UpToDate reports whether an apply would change nothing.
func (pl Plan) UpToDate() bool { return len(pl.Pending()) == 0 && len(pl.Removes) == 0 }

// Render works out what an apply writes, reading but changing nothing.
func Render(p paths.Paths, m *manifest.Manifest, getenv func(string) string) (Plan, error) {
	signers, problems := Signers(m, p.SSHDir)
	pl := Plan{Signers: signers, Problems: problems, Global: GlobalConfigPath(p, getenv)}
	allowed := AllowedSignersPath(p)
	keep := map[string]bool{}
	for _, s := range signers {
		path := FragmentPath(p, s.Profile)
		keep[path] = true
		pl.Writes = append(pl.Writes, Write{Path: path, Old: read(path), New: Fragment(s, allowed), Mode: 0o600})
	}
	pl.Writes = append(pl.Writes, Write{Path: allowed, Old: read(allowed), New: AllowedSigners(signers), Mode: 0o600})
	if entries, err := os.ReadDir(Dir(p)); err == nil {
		for _, e := range entries {
			path := filepath.Join(Dir(p), e.Name())
			if strings.HasSuffix(e.Name(), ".gitconfig") && !e.IsDir() && !keep[path] {
				pl.Removes = append(pl.Removes, path)
			}
		}
	}
	old := read(pl.Global)
	block := Block(p, signers)
	pl.Writes = append(pl.Writes, Write{Path: pl.Global, Old: old, New: ReplaceBlock(old, block), Mode: 0o644})
	return pl, nil
}

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// Fragment is a profile's git config fragment.
func Fragment(s Signer, allowedSigners string) string {
	var b strings.Builder
	fmt.Fprintf(&b, fragmentHeader, s.Profile)
	b.WriteString("[user]\n")
	fmt.Fprintf(&b, "\temail = %s\n", s.Git.Email)
	if s.Git.Name != "" {
		fmt.Fprintf(&b, "\tname = %q\n", s.Git.Name)
	}
	if s.signs() {
		if s.OnePassword {
			// The private half is in 1Password: git hands the public key to
			// op-ssh-sign, which asks the app to sign.
			fmt.Fprintf(&b, "\tsigningkey = %q\n", "key::"+s.PubLine)
		} else {
			fmt.Fprintf(&b, "\tsigningkey = %s\n", configPath(s.PubPath))
		}
		b.WriteString("[gpg]\n\tformat = ssh\n")
		b.WriteString("[gpg \"ssh\"]\n")
		fmt.Fprintf(&b, "\tallowedSignersFile = %s\n", configPath(allowedSigners))
		if s.OnePassword {
			fmt.Fprintf(&b, "\tprogram = %s\n", configPath(OnePasswordSigner(runtime.GOOS)))
		}
		b.WriteString("[commit]\n\tgpgsign = true\n[tag]\n\tgpgsign = true\n")
	}
	return b.String()
}

// OnePasswordSigner is where 1Password installs op-ssh-sign on goos.
func OnePasswordSigner(goos string) string {
	switch goos {
	case "darwin":
		return "/Applications/1Password.app/Contents/MacOS/op-ssh-sign"
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "1Password", "app", "8", "op-ssh-sign.exe")
	}
	return "/opt/1Password/op-ssh-sign"
}

// configPath writes a path git reads: forward slashes, quoted when it holds a space.
func configPath(p string) string {
	p = filepath.ToSlash(p)
	if strings.ContainsAny(p, " \t#;") {
		return fmt.Sprintf("%q", p)
	}
	return p
}

// AllowedSigners is the allowed_signers file: one line per signing profile.
func AllowedSigners(signers []Signer) string {
	var lines []string
	for _, s := range signers {
		if s.signs() {
			lines = append(lines, fmt.Sprintf("%s namespaces=\"git\" %s", s.Git.Email, s.PubLine))
		}
	}
	sort.Strings(lines)
	head := "# Written by `authama ssh git sign apply`: who may sign commits, for git's signature checks.\n"
	if len(lines) == 0 {
		return head
	}
	return head + strings.Join(lines, "\n") + "\n"
}

// Block is the managed includeIf block for the global git config; "" when no
// profile claims a directory.
func Block(p paths.Paths, signers []Signer) string {
	var b strings.Builder
	for _, s := range signers {
		for _, d := range s.Git.Dirs {
			fmt.Fprintf(&b, "[includeIf \"gitdir:%s\"]\n\tpath = %s\n", manifest.GitDir(d), configPath(FragmentPath(p, s.Profile)))
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return BeginMarker + "\n" + b.String() + EndMarker + "\n"
}

// ReplaceBlock puts block in place of the managed block in config, or appends
// it; an empty block removes the managed one. Everything outside the markers
// is kept byte for byte.
func ReplaceBlock(config, block string) string {
	start := strings.Index(config, BeginMarker)
	if start >= 0 {
		rest := config[start:]
		end := strings.Index(rest, EndMarker)
		if end >= 0 {
			after := rest[end+len(EndMarker):]
			after = strings.TrimPrefix(after, "\n")
			return config[:start] + block + after
		}
	}
	if block == "" {
		return config
	}
	if config != "" && !strings.HasSuffix(config, "\n") {
		config += "\n"
	}
	if config != "" {
		config += "\n"
	}
	return config + block
}

// Result is what an apply did.
type Result struct {
	Written []string
	Removed []string
	// Backup is the copy of the global git config taken before it changed.
	Backup string
}

// Apply makes pl: the fragments and allowed_signers owner-only, the global git
// config with its own mode and through its symlink, after a copy of it is kept
// under the state home's backups/gitconfig/.
func Apply(p paths.Paths, pl Plan, now time.Time) (Result, error) {
	var res Result
	if err := fs.EnsureDir(Dir(p), 0o700); err != nil {
		return res, err
	}
	for _, w := range pl.Pending() {
		target, mode := w.Path, w.Mode
		if w.Path == pl.Global {
			resolved, err := resolveLink(w.Path)
			if err != nil {
				return res, err
			}
			target = resolved
			if fi, err := os.Stat(target); err == nil {
				mode = fi.Mode().Perm()
			}
			if w.Old != "" {
				backup := filepath.Join(p.State(), "backups", "gitconfig", now.UTC().Format("20060102T150405Z")+".gitconfig")
				if err := fs.WriteTextAtomic(backup, w.Old, 0o600); err != nil {
					return res, fmt.Errorf("keep a copy of %s first: %w", w.Path, err)
				}
				res.Backup = backup
			}
		}
		if err := fs.WriteTextAtomic(target, w.New, mode); err != nil {
			return res, err
		}
		res.Written = append(res.Written, w.Path)
	}
	for _, path := range pl.Removes {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return res, err
		}
		res.Removed = append(res.Removed, path)
	}
	if len(res.Written)+len(res.Removed) > 0 {
		log.Audit(p.AuditLog(), "git.sign.apply", log.Field{Key: "written", Value: len(res.Written)},
			log.Field{Key: "removed", Value: len(res.Removed)}, log.Field{Key: "signers", Value: len(pl.Signers)})
	}
	return res, nil
}

// resolveLink follows a symlinked file to the file it names, so a write
// replaces the content and keeps the link.
func resolveLink(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	return filepath.EvalSymlinks(path)
}
