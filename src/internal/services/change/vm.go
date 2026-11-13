package change

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/services/vmsync"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/util/scheduler"
)

// NewSyncer builds the syncer the VM changes use; tests swap it.
var NewSyncer = func(p paths.Paths, cfg settings.VMConfig, opt vmsync.Options) *vmsync.Syncer {
	return vmsync.New(p, cfg, opt)
}

// VMSync is `authama ssh vm sync --yes [--repin]`.
//
// The manifest half - removing the hosts of gone VMs, adding declared
// devboxes - runs in Run, against the copy, so the preview shows the exact
// manifest and config diff. The half outside the home - pins, the guest's
// authorized_keys, Docker contexts - is listed in Effects and made in
// OnApply: a hashed known_hosts line gets a fresh salt every time it is
// written, so a pin made in the copy could never hash the same twice.
// OnApply backs the manifest and inventory up first.
func VMSync(repin bool) Change {
	args := []string{"--yes"}
	if repin {
		args = append(args, "--repin")
	}
	var cfg settings.VMConfig
	syncer := func(p paths.Paths, probe bool) *vmsync.Syncer {
		opt := vmsync.DefaultOptions(nil)
		opt.Probe = probe
		opt.Repin = opt.Repin || repin
		if !probe {
			opt.Docker = false
		}
		return NewSyncer(p, cfg, opt)
	}
	return Change{
		Command: "vm sync", Args: args,
		Effects: func(p paths.Paths) ([]string, error) {
			cfg, _ = settings.LoadVMConfig(p.SettingsFile())
			s := syncer(p, true)
			plan, err := s.Plan()
			if err != nil {
				return nil, err
			}
			var out []string
			if plan.ManifestChanges() {
				out = append(out, "backs up manifest.json and inventory.json to "+s.BackupDir())
			}
			for _, a := range plan.Actions {
				if a.Outside() {
					out = append(out, a.Detail)
				}
			}
			for _, n := range plan.Notes {
				out = append(out, "note: "+n)
			}
			if len(out) == 0 {
				out = append(out, "changes nothing outside the manifest")
			}
			return out, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			s := syncer(p, true)
			plan, err := s.Plan()
			if err != nil {
				return err
			}
			if plan.ManifestChanges() {
				if _, err := s.BackupManifest(); err != nil {
					return fmt.Errorf("back up the manifest: %w (nothing changed)", err)
				}
			}
			_, err = s.ApplyOutside(plan, out)
			return err
		},
		Run: func(p paths.Paths, out io.Writer) error {
			s := syncer(p, false)
			plan, err := s.Plan()
			if err != nil {
				return err
			}
			if !plan.ManifestChanges() {
				_, _ = fmt.Fprintln(out, "manifest: nothing to change")
				return nil
			}
			_, aerr := s.ApplyManifest(plan, out)
			return errors.Join(aerr, ApplyManifestEdit(out, p))
		},
	}
}

// VMAdoptKey is `authama ssh vm adopt-key <key> <profile> <alias>
// --hypervisor <hv> --vm <vm> --user <user> --yes`: add the VM host and copy
// the loose ~/.ssh/<key> pair into the profile under the key's own name. The
// copy holds the private key only as a placeholder, which the preview copies
// in its place; the original is never removed from the web.
func VMAdoptKey(key, profile, alias, hv, vmName, user string) Change {
	return Change{
		Command: "vm adopt-key",
		Args:    []string{key, profile, alias, "--hypervisor=" + hv, "--vm=" + vmName, "--user=" + user, "--yes"},
		Subjects: []history.Subject{{Kind: history.KindProfile, Name: profile}, {Kind: history.KindHost, Name: alias},
			{Kind: history.KindKey, Name: profile + "/" + key}},
		Effects: func(p paths.Paths) ([]string, error) {
			if err := CheckAdopt(p, key, profile, alias, hv, user); err != nil {
				return nil, err
			}
			return []string{
				"backs up manifest.json and inventory.json to " + filepath.Join(p.State(), "backups", "vmsync"),
				"copies ~/.ssh/" + key + " and " + key + ".pub into profile " + profile + "; the originals stay",
			}, nil
		},
		OnApply: func(p paths.Paths, _ io.Writer) error {
			_, err := NewSyncer(p, settings.VMConfig{}, vmsync.Options{}).BackupManifest()
			return err
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if err := CheckAdopt(p, key, profile, alias, hv, user); err != nil {
				return err
			}
			if err := vmsync.AddVMHost(p, profile, alias, hv, vmName, user, key); err != nil {
				return err
			}
			res, err := vmsync.AdoptKey(p, filepath.Join(p.SSHDir, key), profile, key, false)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "adopted ~/.ssh/%s as %s\n", key, res.Dest)
			if released, err := vmsync.ReleaseReservation(p, filepath.Join(p.SSHDir, key)); err != nil {
				return err
			} else if released {
				_, _ = fmt.Fprintf(out, "released the reservation of %s: it has a host now\n", key)
			}
			if err := ApplyManifestEdit(out, p); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "next: `authama ssh vm sync` pins %s's host key and installs the key in the guest if it lacks it\n", alias)
			return nil
		},
	}
}

// notKeyNames are files in ~/.ssh that are never a key to adopt.
var notKeyNames = map[string]bool{"config": true, "known_hosts": true, "known_hosts.old": true, "authorized_keys": true}

// CheckAdopt validates an adoption against the home as it is now: a loose
// pair in ~/.ssh, a hypervisor authama knows that does not manage its own
// keys, a guest user it will embed, a per_service profile, a new alias.
func CheckAdopt(p paths.Paths, key, profile, alias, hv, user string) error {
	if key == "" || filepath.Base(key) != key || notKeyNames[key] || filepath.Ext(key) == ".pub" {
		return fmt.Errorf("%q: name a key file directly in ~/.ssh, without .pub", key)
	}
	for _, f := range []string{key, key + ".pub"} {
		if _, err := os.Stat(filepath.Join(p.SSHDir, f)); err != nil {
			return fmt.Errorf("~/.ssh/%s: %w", f, err)
		}
	}
	known := false
	for _, n := range manifest.KnownHypervisors() {
		known = known || n == hv
	}
	switch {
	case !known:
		return fmt.Errorf("unknown hypervisor %q (known: %v)", hv, manifest.KnownHypervisors())
	case hv == manifest.HypervisorOrbStack || hv == manifest.HypervisorPodman:
		return fmt.Errorf("%s manages its own SSH keys; there is nothing to adopt a key onto", hv)
	case !settings.ValidUser(user):
		return fmt.Errorf("guest user %q: letters, digits, '.', '_' and '-'", user)
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	if prof, ok := m.Profiles[profile]; ok && prof.KeyScope == "shared" {
		return fmt.Errorf("profile %s shares one key across its hosts; adopt into a per_service profile", profile)
	}
	for _, pn := range m.ProfileNames() {
		for _, h := range m.Profiles[pn].Hosts {
			if h.Alias == alias {
				return fmt.Errorf("host %s already exists in profile %s", alias, pn)
			}
		}
	}
	return nil
}

// VMSchedule is `authama ssh vm sync --install-schedule --yes` (install) or
// `--remove-schedule --yes`: the daily `vm sync --quiet --yes` job.
func VMSchedule(install bool) Change {
	flag := "--remove-schedule"
	if install {
		flag = "--install-schedule"
	}
	return Change{
		Command: "vm sync", Args: []string{flag, "--yes"}, NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			if !install {
				return []string{"removes the scheduled VM sync (" + scheduler.VMSyncLabel + ")"}, nil
			}
			if p.IsDev() {
				return nil, fmt.Errorf("installing a schedule is refused in dev mode: it registers a job with " +
					"launchd, systemd or schtasks, which is machine-wide and outlives the sandbox")
			}
			return []string{"registers a daily job with this system's scheduler: " + scheduler.VMSyncCommand()}, nil
		},
		OnApply: func(_ paths.Paths, out io.Writer) error {
			if !install {
				scheduler.Remove(scheduler.VMSyncLabel)
				_, _ = fmt.Fprintln(out, "removed the scheduled VM sync")
				return nil
			}
			command := scheduler.VMSyncCommand()
			if err := scheduler.Install(command, scheduler.VMSyncLabel); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "installed the scheduled VM sync: %s\n", command)
			return nil
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}
