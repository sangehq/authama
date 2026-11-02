package vmsync

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/lifecycle"
	"github.com/simtabi/authama/src/internal/services/reconciler"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/util/perms"
)

// BackupDir is where sync keeps what it backs up before removing: the
// manifest and inventory, and exported Docker contexts.
func (s *Syncer) BackupDir() string { return filepath.Join(s.P.State(), "backups", "vmsync") }

// ManifestChanges reports whether the plan edits the manifest.
func (p Plan) ManifestChanges() bool {
	for _, a := range p.Actions {
		if !a.Outside() {
			return true
		}
	}
	return false
}

// BackupManifest copies manifest.json and inventory.json into BackupDir and
// returns the manifest copy's path.
func (s *Syncer) BackupManifest() (string, error) {
	dir := s.BackupDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	var first string
	for _, src := range []string{s.P.Manifest(), s.P.Inventory()} {
		b, err := os.ReadFile(src)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		base := strings.TrimSuffix(filepath.Base(src), ".json")
		dst := filepath.Join(dir, base+"-"+stamp+".json")
		if err := os.WriteFile(dst, b, 0o600); err != nil {
			return "", err
		}
		if first == "" {
			first = dst
		}
	}
	return first, nil
}

// ApplyManifest makes the plan's manifest steps: removing gone hosts, adding
// and updating declared devboxes, adopting declared keys. The caller holds
// the lock, has snapshotted ~/.ssh and backed the manifest up, and re-renders
// the config afterwards.
func (s *Syncer) ApplyManifest(plan Plan, out io.Writer) (int, error) {
	var errs []error
	done := 0
	for _, a := range plan.Actions {
		if a.Outside() {
			continue
		}
		var err error
		switch a.Kind {
		case ActRemoveHost:
			_, err = lifecycle.New(s.P, platform.EmitUseKeychain()).DeleteHost(a.Profile, a.Alias, lifecycle.Options{})
		case ActAddDevbox:
			err = AddVMHost(s.P, a.Profile, a.Alias, a.devbox.Hypervisor, a.devbox.VM, a.devbox.User, a.devbox.Key)
		case ActUpdateDevbox:
			err = s.updateDevbox(a)
		case ActAdoptKey:
			if _, err = AdoptKey(s.P, a.keySrc, a.Profile, a.keyName, false); err == nil {
				_, err = ReleaseReservation(s.P, a.keySrc)
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Detail, err))
			_, _ = fmt.Fprintf(out, "failed: %s: %v\n", a.Detail, err)
			continue
		}
		done++
		_, _ = fmt.Fprintln(out, "done: "+a.Detail)
	}
	return done, errors.Join(errs...)
}

func (s *Syncer) updateDevbox(a Action) error {
	db := a.devbox
	f := editor.HostFields{User: &db.User, VM: &manifest.VMSpec{Hypervisor: db.Hypervisor, Name: db.VM}}
	if db.Key != "" {
		f.KeyName = &db.Key
	}
	return editor.New(s.P).EditHost(a.Profile, a.Alias, f)
}

// ApplyOutside makes the plan's steps outside the manifest: pins, key
// installs, Docker contexts. Each is independent; a failure is reported and
// the rest still run.
func (s *Syncer) ApplyOutside(plan Plan, out io.Writer) (int, error) {
	var errs []error
	done := 0
	kh := knownhosts.New(s.P.SSHDir)
	for _, a := range plan.Actions {
		if !a.Outside() {
			continue
		}
		var line string
		var err error
		switch a.Kind {
		case ActPin:
			_, err = kh.PinAs(a.token, a.scanned)
		case ActRepin:
			var untagged int
			if _, untagged, err = kh.UnpinTagged(a.token); err == nil {
				if untagged > 0 {
					err = fmt.Errorf("known_hosts also holds %d line(s) for %s that authama did not write; remove them with: ssh-keygen -R %s", untagged, a.token, a.token)
				} else {
					_, err = kh.PinAs(a.token, a.scanned)
				}
			}
		case ActInstallKey:
			err = installKey(a)
		case ActDockerCreate, ActDockerUpdate:
			line, err = s.Deps.Docker.Apply(a.Alias)
		case ActDockerRemove:
			line, err = s.Deps.Docker.Remove(a.context, s.BackupDir())
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Detail, err))
			_, _ = fmt.Fprintf(out, "failed: %s: %v\n", a.Detail, err)
			continue
		}
		done++
		if line == "" {
			line = a.Detail
		}
		_, _ = fmt.Fprintln(out, "done: "+line)
	}
	return done, errors.Join(errs...)
}

// installScript appends pub to user's authorized_keys in a POSIX guest, once.
// Both values are validated before they get here (a user name of letters,
// digits, '.', '_' and '-'; a key line with no quote or backslash), so single
// quotes hold them.
func installScript(user, pub string) string {
	return "set -e\n" +
		"u='" + user + "'; k='" + pub + "'\n" +
		`h=$(getent passwd "$u" 2>/dev/null | cut -d: -f6)` + "\n" +
		`[ -n "$h" ] || h=$(eval echo "~$u")` + "\n" +
		`[ -d "$h" ] || { echo "no home directory for $u" >&2; exit 3; }` + "\n" +
		`mkdir -p "$h/.ssh"; touch "$h/.ssh/authorized_keys"` + "\n" +
		`grep -qxF "$k" "$h/.ssh/authorized_keys" || printf '%s\n' "$k" >> "$h/.ssh/authorized_keys"` + "\n" +
		`chmod 700 "$h/.ssh"; chmod 600 "$h/.ssh/authorized_keys"` + "\n" +
		`chown "$u" "$h/.ssh" "$h/.ssh/authorized_keys" 2>/dev/null || true` + "\n" +
		"echo authama-key-installed\n"
}

func installKey(a Action) error {
	out, err := a.driver.Exec(a.vm, "sh", "-c", installScript(a.user, a.pubLine))
	if err != nil {
		return fmt.Errorf("%v (install it by hand: append the .pub to ~%s/.ssh/authorized_keys in the guest)", err, a.user)
	}
	if !strings.Contains(string(out), "authama-key-installed") {
		return fmt.Errorf("the guest did not confirm the install (a non-POSIX guest?); append the .pub to ~%s/.ssh/authorized_keys by hand", a.user)
	}
	return nil
}

// AddVMHost adds a VM host to profile (created per_service when missing),
// using keyName when set. It does not re-render the config.
func AddVMHost(p paths.Paths, profile, alias, hv, vmName, user, keyName string) error {
	ed := editor.New(p)
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	if _, ok := m.Profiles[profile]; !ok {
		if err := ed.AddProfile(profile, "per_service", nil); err != nil {
			return err
		}
	}
	host := alias
	f := editor.HostFields{Hostname: &host, User: &user, VM: &manifest.VMSpec{Hypervisor: hv, Name: vmName}}
	if keyName != "" {
		f.KeyName = &keyName
	}
	return ed.AddHost(profile, alias, f)
}

// AdoptResult is what AdoptKey did.
type AdoptResult struct {
	Dest     string
	Recorded bool
	Moved    bool
}

// AdoptKey copies the loose keypair src (and src.pub) into profile as
// keyName, records it in the inventory, and - only when move is set and the
// copy is verified byte-identical - removes the originals. It refuses to
// overwrite a key already in the profile.
func AdoptKey(P paths.Paths, src, profile, keyName string, move bool) (AdoptResult, error) {
	m, err := manifest.Load(P.Manifest())
	if err != nil {
		return AdoptResult{}, err
	}
	ref := manifest.KeyRef{Profile: profile, KeyName: keyName}
	dst := m.KeyPath(P.SSHDir, ref)
	res := AdoptResult{Dest: dst}
	if fileExists(dst) {
		return res, fmt.Errorf("%s already holds a key; authama will not overwrite it", dst)
	}
	if !fileExists(src) || !fileExists(src+".pub") {
		return res, fmt.Errorf("%s and %s.pub must both exist", src, src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), perms.DirMode); err != nil {
		return res, err
	}
	_ = perms.SetPerms(filepath.Dir(dst), perms.DirMode)
	for _, pair := range [][3]any{{src, dst, perms.PrivateKeyMode}, {src + ".pub", dst + ".pub", perms.PublicKeyMode}} {
		if err := copyExact(pair[0].(string), pair[1].(string), pair[2].(os.FileMode)); err != nil {
			_ = os.Remove(dst)
			_ = os.Remove(dst + ".pub")
			return res, err
		}
	}
	inv, err := inventory.Load(P.Inventory())
	if err != nil {
		return res, err
	}
	res.Recorded, err = reconciler.New(P, m, inv, platform.EmitUseKeychain()).RecordExisting(ref)
	if err != nil {
		return res, err
	}
	if move {
		if !sameFile(src, dst) || !sameFile(src+".pub", dst+".pub") {
			return res, fmt.Errorf("the copy of %s does not match; the original is kept", src)
		}
		if err := os.Remove(src); err != nil {
			return res, err
		}
		if err := os.Remove(src + ".pub"); err != nil {
			return res, err
		}
		res.Moved = true
	}
	return res, nil
}

// ReleaseReservation drops the reservation of a loose key that has just been
// adopted from src (a path directly in ~/.ssh): the key now has a host, so the
// note that it is kept for later no longer applies. Reports whether one was
// dropped.
func ReleaseReservation(P paths.Paths, src string) (bool, error) {
	if filepath.Dir(filepath.Clean(src)) != filepath.Clean(P.SSHDir) {
		return false, nil
	}
	return editor.New(P).UnreserveKey(filepath.Base(src), true)
}

func copyExact(src, dst string, mode os.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return perms.SetPerms(dst, mode)
}

func sameFile(a, b string) bool {
	x, err1 := os.ReadFile(a)
	y, err2 := os.ReadFile(b)
	return err1 == nil && err2 == nil && string(x) == string(y)
}
