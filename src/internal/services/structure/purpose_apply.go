package structure

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/reconciler"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/perms"
)

// runSange is the sange CLI used to retarget user.signingkey after a
// git-signing key moves. Tests replace it. Authama never writes ~/.gitconfig.
var runSange = defaultRunSange

func defaultRunSange(args ...string) (string, error) {
	bin, err := exec.LookPath("sange")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// SangeRetargetArgs is the handoff command that updates signing.key through
// sange. Authama does not edit gitconfig itself.
func SangeRetargetArgs(from, to string) []string {
	return []string{"profile", "signing", "retarget", "--from", from, "--to", to}
}

func applyPurposeMoves(p paths.Paths, moves []Move, out io.Writer, res *Result) error {
	var purpose, copies []Move
	for _, mv := range moves {
		switch mv.Kind {
		case MovePurpose:
			purpose = append(purpose, mv)
		case MoveCopyRemove:
			copies = append(copies, mv)
		}
	}
	if len(purpose) == 0 && len(copies) == 0 {
		return nil
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	backup := BackupDir(p, time.Now())
	for _, mv := range purpose {
		if err := applyOnePurpose(p, m, mv, backup, out, res); err != nil {
			return err
		}
	}
	if len(purpose) > 0 {
		if err := m.Validate(); err != nil {
			return err
		}
		if err := m.Save(p.Manifest()); err != nil {
			return err
		}
		inv, err := inventory.Load(p.Inventory())
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			r := reconciler.New(p, m, inv, platform.EmitUseKeychain())
			for _, mv := range purpose {
				if _, err := r.RecordExisting(manifest.KeyRef{Profile: mv.Profile, KeyName: mv.Name}); err != nil {
					return err
				}
			}
		}
	}
	for _, mv := range copies {
		if err := applyCopyRemove(p, m, mv, backup, out, res); err != nil {
			return err
		}
	}
	return nil
}

func applyOnePurpose(p paths.Paths, m *manifest.Manifest, mv Move, backup string, out io.Writer, res *Result) error {
	if err := manifest.CheckToolSlug(mv.Tool); err != nil {
		return fmt.Errorf("%s: %w", mv.Line(), err)
	}
	ref := manifest.KeyRef{Profile: mv.Profile, KeyName: mv.Name}
	src := m.KeyPath(p.SSHDir, ref)
	if err := checkPair(src); err != nil {
		return fmt.Errorf("%s: %w", mv.Line(), err)
	}
	kind := mv.ToolKind
	if kind == "" {
		kind = mv.Tool
	}
	// Update the spec first so dest uses the new layout, then move files.
	prof := m.Profiles[mv.Profile]
	spec, ok := m.KeySpecFor(ref)
	if !ok {
		spec = manifest.KeySpec{Name: mv.Name}
	}
	if prof.InOnePassword() {
		return fmt.Errorf("%s: a 1Password profile's keys are not moved as files", mv.Line())
	}
	spec.Role = manifest.RoleTool
	spec.Tool = mv.Tool
	spec.ToolKind = kind
	spec.Path = ""
	replaced := false
	for i, k := range prof.Keys {
		if k.Name == mv.Name {
			prof.Keys[i] = spec
			replaced = true
			break
		}
	}
	if !replaced {
		prof.Keys = append(prof.Keys, spec)
	}
	m.SetProfile(mv.Profile, prof)
	dest := m.KeyPath(p.SSHDir, ref)
	if err := os.MkdirAll(filepath.Dir(dest), perms.DirMode); err != nil {
		return err
	}
	if src != dest {
		if err := backupPair(p, src, backup); err != nil {
			return fmt.Errorf("backing up %s: %w", mv.Line(), err)
		}
		res.Backup = backup
		if err := movePair(src, dest); err != nil {
			return fmt.Errorf("%s: %w", mv.Line(), err)
		}
		from := "~/.ssh/" + relSSH(p, src)
		to := "~/.ssh/" + relSSH(p, dest)
		res.Moved = append(res.Moved, from+" -> "+to)
		changed, err := rewriteRefs(p, src, dest)
		if err != nil {
			return err
		}
		res.Rewrote = append(res.Rewrote, changed...)
		_, _ = fmt.Fprintf(out, "moved %s -> %s\n", from, to)
		audit(p, mv, from, to)
	}
	res.Declared = append(res.Declared, mv.Profile+"/"+mv.Name)
	if kind == manifest.ToolKindGitSigning || mv.Tool == manifest.ToolGit {
		from := src + ".pub"
		to := dest + ".pub"
		cmd := SangeRetargetArgs(from, to)
		// Prefer ~-form for the command the owner may re-run.
		fromShow, toShow := "~/.ssh/"+relSSH(p, from), "~/.ssh/"+relSSH(p, to)
		cmd = SangeRetargetArgs(fromShow, toShow)
		msg, err := runSange(cmd...)
		if err != nil {
			line := "sange " + strings.Join(cmd, " ")
			res.Handoff = append(res.Handoff, line)
			_, _ = fmt.Fprintf(out, "sange signing retarget not applied (%v); run: %s\n", err, line)
			if strings.TrimSpace(msg) != "" {
				_, _ = fmt.Fprintf(out, "%s\n", strings.TrimSpace(msg))
			}
		} else {
			_, _ = fmt.Fprintf(out, "sange profile signing retarget updated the signing key\n")
			if strings.TrimSpace(msg) != "" {
				_, _ = fmt.Fprintf(out, "%s\n", strings.TrimSpace(msg))
			}
		}
	}
	return nil
}

func applyCopyRemove(p paths.Paths, m *manifest.Manifest, mv Move, backup string, out io.Writer, res *Result) error {
	src := srcAbs(p, mv.Src)
	if src == "" {
		return fmt.Errorf("%s: a path outside ~/.ssh", mv.Line())
	}
	if err := checkPair(src); err != nil {
		return fmt.Errorf("%s: %w", mv.Line(), err)
	}
	if remaining := copyRemoveRefs(p, src); len(remaining) > 0 {
		return fmt.Errorf("%s: refused; still referenced by %s", mv.Line(), strings.Join(remaining, ", "))
	}
	// Must be a leftover copy of a managed key (same .pub fingerprint).
	pub, err := os.ReadFile(src + ".pub")
	if err != nil {
		return fmt.Errorf("%s: %w", mv.Line(), err)
	}
	fp, err := fingerprintOfPub(src + ".pub")
	if err != nil {
		return fmt.Errorf("%s: %w", mv.Line(), err)
	}
	if !managedHasFingerprint(p, m, fp) {
		return fmt.Errorf("%s: not a leftover copy of a key authama manages (fingerprint %s)", mv.Line(), fp)
	}
	_ = pub
	if err := backupPair(p, src, backup); err != nil {
		return fmt.Errorf("backing up %s: %w", mv.Line(), err)
	}
	res.Backup = backup
	for _, suffix := range []string{"", ".pub", "-cert.pub"} {
		f := src + suffix
		if _, err := os.Lstat(f); err != nil {
			continue
		}
		if err := os.Remove(f); err != nil {
			return fmt.Errorf("%s: %w", mv.Line(), err)
		}
	}
	from := "~/.ssh/" + relSSH(p, src)
	res.Moved = append(res.Moved, from+" -> (removed, backup in "+backup+")")
	_, _ = fmt.Fprintf(out, "removed leftover %s (backed up)\n", from)
	audit(p, mv, from, "")
	return nil
}

func copyRemoveRefs(p paths.Paths, abs string) []string {
	var out []string
	if files := ConfigRefs(p)[abs]; len(files) > 0 {
		out = append(out, files...)
	}
	for _, g := range signingKeyPaths(p) {
		if sameFile(g, abs) || sameFile(g, abs+".pub") {
			out = append(out, "user.signingkey")
		}
	}
	return unique(out)
}

func managedHasFingerprint(p paths.Paths, m *manifest.Manifest, fp string) bool {
	if all, err := m.KeyRefs(); err == nil {
		for _, ref := range all {
			pub := m.KeyPath(p.SSHDir, ref) + ".pub"
			got, err := fingerprintOfPub(pub)
			if err == nil && got == fp {
				return true
			}
		}
	}
	return false
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
