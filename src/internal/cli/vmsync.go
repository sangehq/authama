package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/hypervisor"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/change"
	"github.com/simtabi/authama/src/internal/services/dockerctx"
	"github.com/simtabi/authama/src/internal/services/keyaudit"
	"github.com/simtabi/authama/src/internal/services/settings"
	"github.com/simtabi/authama/src/internal/services/vmsync"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/util/scheduler"
)

// vmConfig is authama.json's hypervisors and devboxes, as start-up read them.
func vmConfig(p paths.Paths) settings.VMConfig {
	if st := currentStartup(); st != nil && st.Settings != nil {
		return st.Settings.VM
	}
	cfg, _ := settings.LoadVMConfig(p.SettingsFile())
	return cfg
}

// newSyncer is a seam for tests.
var newSyncer = func(p paths.Paths, cfg settings.VMConfig, opt vmsync.Options) *vmsync.Syncer {
	return vmsync.New(p, cfg, opt)
}

// printPlan writes a plan's hosts, actions and notes.
func printPlan(c *cobra.Command, plan vmsync.Plan) {
	out := c.OutOrStdout()
	if len(plan.Hosts) == 0 {
		_, _ = fmt.Fprintln(out, "no VM hosts in the manifest (add one with `"+brand.Command+" ssh vm add <profile> <alias>`)")
	}
	for _, h := range plan.Hosts {
		note := ""
		if h.Note != "" {
			note = "  - " + h.Note
		}
		decl := ""
		if h.Declared {
			decl = " (devbox)"
		}
		_, _ = fmt.Fprintf(out, "  %-24s %-10s %-28q %s%s%s\n", h.Profile+"/"+h.Alias, h.Hypervisor, h.VM, h.State, decl, note)
	}
	if plan.Empty() {
		_, _ = fmt.Fprintln(out, "nothing to do: every VM host is in step")
	} else {
		_, _ = fmt.Fprintln(out, "plan:")
		for _, l := range plan.Lines() {
			_, _ = fmt.Fprintln(out, "  - "+l)
		}
	}
	for _, n := range plan.Notes {
		_, _ = fmt.Fprintln(out, "note: "+n)
	}
}

// runVMSync applies a plan. The caller has confirmed and holds the lock. It
// backs up and edits the manifest first, then plans again so a devbox it just
// added is pinned, keyed and given a Docker context in the same run.
func runVMSync(out io.Writer, p paths.Paths, s *vmsync.Syncer, plan vmsync.Plan, quiet bool) error {
	var errs []error
	if plan.ManifestChanges() {
		snapshotBeforeMutation(p)
		bak, err := s.BackupManifest()
		if err != nil {
			return fmt.Errorf("back up the manifest: %w (nothing changed)", err)
		}
		if !quiet && bak != "" {
			_, _ = fmt.Fprintf(out, "backed up the manifest to %s\n", bak)
		}
		if _, err := s.ApplyManifest(plan, out); err != nil {
			errs = append(errs, err)
		}
		w := out
		if quiet {
			w = io.Discard
		}
		if err := change.ApplyManifestEdit(w, p); err != nil {
			errs = append(errs, err)
		}
		if plan, err = s.Plan(); err != nil {
			return errors.Join(append(errs, err)...)
		}
	}
	if _, err := s.ApplyOutside(plan, out); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func newVMSyncCmd() *cobra.Command {
	var dryRun, yes, quiet, repin, noProbe, installSchedule, removeSchedule bool
	c := &cobra.Command{
		Use:   "sync",
		Short: "Keep VM hosts in step with their hypervisors: pins, guest keys, Docker contexts, gone VMs",
		Long: "For every VM host, ask its hypervisor what became of the VM:\n" +
			"  running    pin its host key when nothing is pinned, re-install the host's public key in\n" +
			"             the guest when the guest refuses it, create or refresh Docker context ssh://<alias>\n" +
			"  gone       back up the manifest, remove the host entry (never its key files) and the\n" +
			"             Docker context authama made for it (exported first)\n" +
			"  suspended  reported and left alone - sync never starts, stops or deletes a VM\n" +
			"It also adds the devboxes authama.json declares, and removes Docker contexts authama made\n" +
			"whose host is gone. Running it twice changes nothing the second time.\n\n" +
			"--install-schedule runs `sync --quiet --yes` daily with launchd, systemd/cron or Task Scheduler.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if installSchedule || removeSchedule {
				return vmSchedule(c, p, installSchedule, yes)
			}
			opt := vmsync.DefaultOptions(nil)
			opt.Probe = !noProbe
			if repin {
				opt.Repin = true
			}
			s := newSyncer(p, vmConfig(p), opt)
			ask := asks(yes) && !dryRun
			if !ask && !dryRun {
				lockForMutation(p)
			}
			plan, err := s.Plan()
			if err != nil {
				return err
			}
			if !quiet || dryRun {
				printPlan(c, plan)
			}
			if dryRun || plan.Empty() {
				if dryRun && !plan.Empty() {
					_, _ = fmt.Fprintln(c.OutOrStdout(), "dry run: nothing changed")
				}
				return nil
			}
			if ask {
				if err := confirmOrAbort(c, "Apply this plan?", false); err != nil {
					return err
				}
				shown := strings.Join(plan.Lines(), "\n")
				if err := recheck(p, shown, func() (string, error) {
					again, err := s.Plan()
					plan = again
					return strings.Join(again.Lines(), "\n"), err
				}); err != nil {
					return err
				}
			}
			return runVMSync(c.OutOrStdout(), p, s, plan, quiet)
		},
	}
	f := c.Flags()
	f.BoolVarP(&dryRun, "dry-run", "n", false, "show the plan and change nothing")
	f.BoolVarP(&yes, "yes", "y", false, "apply without confirming (implied when there is no terminal)")
	f.BoolVarP(&quiet, "quiet", "q", false, "print only what changed and errors (for a scheduled run)")
	f.BoolVar(&repin, "repin", false, "replace a VM host's pin when the guest's host key changed (vm.repin_changed)")
	f.BoolVar(&noProbe, "no-probe", false, "ask the hypervisors only; do not connect to running guests")
	f.BoolVar(&installSchedule, "install-schedule", false, "run a quiet sync daily (sync --quiet --yes) with this system's scheduler")
	f.BoolVar(&removeSchedule, "remove-schedule", false, "remove the scheduled sync")
	c.MarkFlagsMutuallyExclusive("install-schedule", "remove-schedule")
	c.MarkFlagsMutuallyExclusive("install-schedule", "dry-run")
	c.MarkFlagsMutuallyExclusive("remove-schedule", "dry-run")
	return c
}

// vmSyncCommand is the scheduled job's command line.
func vmSyncCommand() string { return scheduler.VMSyncCommand() }

func vmSchedule(c *cobra.Command, p paths.Paths, install, yes bool) error {
	if !install {
		if err := confirmChange(c, "remove the scheduled VM sync ("+scheduler.VMSyncLabel+")", yes); err != nil {
			return err
		}
		lockForMutation(p)
		scheduler.Remove(scheduler.VMSyncLabel)
		_, _ = fmt.Fprintln(c.OutOrStdout(), "removed the scheduled VM sync")
		return nil
	}
	if err := refuseInDevMode(p, "vm sync --install-schedule",
		"it registers a job with launchd, systemd or schtasks, which is machine-wide and outlives the sandbox"); err != nil {
		return err
	}
	command := vmSyncCommand()
	if err := confirmChange(c, "vm sync --install-schedule registers a daily job with this system's scheduler:\n  "+command, yes); err != nil {
		return err
	}
	lockForMutation(p)
	if err := scheduler.Install(command, scheduler.VMSyncLabel); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.OutOrStdout(), "installed the scheduled VM sync: %s\n", command)
	return nil
}

// pick asks the person at the terminal to choose one of options by number.
func pick(c *cobra.Command, r *bufio.Reader, title string, options []string) (int, error) {
	out := c.OutOrStdout()
	_, _ = fmt.Fprintln(out, title)
	for i, o := range options {
		_, _ = fmt.Fprintf(out, "  %d) %s\n", i+1, o)
	}
	_, _ = fmt.Fprint(out, "number: ")
	line, _ := r.ReadString('\n')
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > len(options) {
		return 0, fmt.Errorf("no choice made: %w", errAborted)
	}
	return n - 1, nil
}

// ask reads one line, offering def.
func askLine(c *cobra.Command, r *bufio.Reader, prompt, def string) string {
	if def != "" {
		prompt += " [" + def + "]"
	}
	_, _ = fmt.Fprint(c.OutOrStdout(), prompt+": ")
	line, _ := r.ReadString('\n')
	if v := strings.TrimSpace(line); v != "" {
		return v
	}
	return def
}

func newVMAdoptKeyCmd() *cobra.Command {
	var hv, vmName, user, keyName string
	var move, docker, dryRun, yes bool
	c := &cobra.Command{
		Use:   "adopt-key [<key> [<profile> [<alias>]]]",
		Short: "Bring a loose ~/.ssh key under management as a devbox VM host's key",
		Long: "adopt-key takes a key you already use for a VM (say ~/.ssh/docker-tunnel), copies it into\n" +
			"a profile as the key of a new VM host, records it, and re-renders ~/.ssh/config. The\n" +
			"original stays where it is unless --move; the private key is copied, never read into\n" +
			"memory for anything else. Run `authama ssh vm sync` afterwards to pin the guest's host\n" +
			"key, install the public key in the guest if it lacks it, and create the Docker context.\n\n" +
			"At a terminal, whatever is missing is asked for: the key from the loose keys in ~/.ssh,\n" +
			"the VM from the enabled hypervisors' VMs (authama.json vm.hypervisors).",
		Args: cobra.MaximumNArgs(3),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			interactive := stdinIsTerminal() && !yes
			r := bufio.NewReader(c.InOrStdin())
			key, profile, alias := argAt(args, 0), argAt(args, 1), argAt(args, 2)
			if key == "" {
				if !interactive {
					return errors.New("name the key to adopt (a file directly in ~/.ssh)")
				}
				m, _ := manifest.Load(p.Manifest())
				res, _ := keyaudit.Reservations(m, vmConfig(p).PlannedKeys())
				loose := keyaudit.MarkReserved(keyaudit.LooseKeysIn(p.SSHDir, manifest.KeyRootAt(p.Manifest())), res)
				var opts []string
				var names []string
				for _, k := range loose {
					if k.CopyOf != "" {
						continue
					}
					label := k.Name
					if k.Comment != "" {
						label += "  (" + k.Comment + ")"
					}
					switch {
					case k.Reserved && k.ReservedFor != "":
						label += "  - reserved for " + k.ReservedFor
					case k.Reserved:
						label += "  - reserved"
					case k.Devbox:
						label += "  - looks like a devbox key"
					}
					opts, names = append(opts, label), append(names, k.Name)
				}
				if len(opts) == 0 {
					return errors.New("no loose keys in ~/.ssh to adopt")
				}
				i, err := pick(c, r, "Which key?", opts)
				if err != nil {
					return err
				}
				key = names[i]
			}
			if strings.ContainsAny(key, `/\`) || strings.HasSuffix(key, ".pub") {
				return fmt.Errorf("%q: name a key file directly in ~/.ssh, without .pub", key)
			}
			src := filepath.Join(p.SSHDir, key)
			if _, err := os.Stat(src); err != nil {
				return fmt.Errorf("~/.ssh/%s: %w", key, err)
			}
			if _, err := os.Stat(src + ".pub"); err != nil {
				return fmt.Errorf("~/.ssh/%s.pub is missing; recreate it with: ssh-keygen -y -f ~/.ssh/%s > ~/.ssh/%s.pub", key, key, key)
			}
			if hv == "" || vmName == "" {
				if !interactive {
					return errors.New("--hypervisor and --vm are required (see `authama ssh vm list`)")
				}
				type choice struct{ hv, vm string }
				var opts []string
				var choices []choice
				for _, d := range hypervisor.Enabled() {
					if d.Name() == manifest.HypervisorOrbStack || d.Name() == manifest.HypervisorPodman ||
						!hypervisor.Available(d) || (hv != "" && d.Name() != hv) {
						continue
					}
					vms, err := d.List()
					if err != nil {
						continue
					}
					for _, v := range vms {
						opts = append(opts, fmt.Sprintf("%-10s %s  (%s)", d.Name(), v.Name, v.State))
						choices = append(choices, choice{d.Name(), v.Name})
					}
				}
				if len(opts) == 0 {
					return errors.New("no VMs found on the enabled hypervisors; pass --hypervisor and --vm")
				}
				i, err := pick(c, r, "Which VM is this key for?", opts)
				if err != nil {
					return err
				}
				hv, vmName = choices[i].hv, choices[i].vm
			}
			if profile == "" {
				profile = "devbox"
				if interactive {
					profile = askLine(c, r, "Profile", profile)
				}
			}
			if alias == "" {
				alias = key
				if interactive {
					alias = askLine(c, r, "Host alias", alias)
				}
			}
			if user == "" && interactive {
				user = askLine(c, r, "Guest user", "")
			}
			if keyName == "" {
				keyName = key
			}
			if err := checkAdopt(p, key, profile, alias, hv, user); err != nil {
				return err
			}
			if d, err := hypervisor.For(hv); err == nil && hypervisor.Available(d) {
				if vms, err := d.List(); err == nil && !hasVM(vms, vmName) {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s does not list a VM %q now; adding it anyway\n", hv, vmName)
				}
			}
			if !hypervisor.IsEnabled(hv) {
				_, _ = fmt.Fprintf(c.ErrOrStderr(), "warning: %s is not in vm.hypervisors; sync will leave this host alone until it is\n", hv)
			}
			summary := fmt.Sprintf("adopt-key will:\n"+
				"  - add VM host %s to profile %s (%s %q, user %s)\n"+
				"  - copy ~/.ssh/%s and %s.pub into the profile as %s and record it\n", alias, profile, hv, vmName, user, key, key, keyName)
			if move {
				summary += "  - remove the originals once the copy is verified identical (--move)\n"
			} else {
				summary += "  - leave ~/.ssh/" + key + " where it is\n"
			}
			if docker {
				summary += "  - create or update Docker context " + alias + " (ssh://" + alias + ")\n"
			}
			if m, err := manifest.Load(p.Manifest()); err == nil {
				if _, ok := m.ReservedKeys[key]; ok {
					summary += "  - release its reservation (authama ssh key reserve)\n"
				}
			}
			summary += "  - back up the manifest, snapshot ~/.ssh, and re-render ~/.ssh/config"
			if dryRun {
				_, _ = fmt.Fprintln(c.OutOrStdout(), summary)
				_, _ = fmt.Fprintln(c.OutOrStdout(), "dry run: nothing changed")
				return nil
			}
			if err := confirmChange(c, summary, yes); err != nil {
				return err
			}
			lockForMutation(p)
			if err := checkAdopt(p, key, profile, alias, hv, user); err != nil {
				return err
			}
			s := newSyncer(p, vmConfig(p), vmsync.Options{})
			snapshotBeforeMutation(p)
			if bak, err := s.BackupManifest(); err != nil {
				return fmt.Errorf("back up the manifest: %w (nothing changed)", err)
			} else if bak != "" {
				_, _ = fmt.Fprintf(c.OutOrStdout(), "backed up the manifest to %s\n", bak)
			}
			if err := vmsync.AddVMHost(p, profile, alias, hv, vmName, user, keyName); err != nil {
				return err
			}
			res, err := vmsync.AdoptKey(p, src, profile, keyName, move)
			if err != nil {
				return fmt.Errorf("host %s was added but the key was not adopted: %w "+
					"(run `authama ssh host delete %s %s` to undo, or fix and adopt again)", alias, err, profile, alias)
			}
			out := c.OutOrStdout()
			_, _ = fmt.Fprintf(out, "adopted ~/.ssh/%s as %s\n", key, res.Dest)
			if released, err := vmsync.ReleaseReservation(p, src); err != nil {
				return err
			} else if released {
				_, _ = fmt.Fprintf(out, "released the reservation of %s: it has a host now\n", key)
			}
			if err := applyManifestEdit(c, p); err != nil {
				return err
			}
			if docker {
				if err := dockerctx.Available(); err != nil {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "docker context skipped: %v\n", err)
				} else if line, err := dockerctx.Apply(alias); err != nil {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "docker context: %v\n", err)
				} else {
					_, _ = fmt.Fprintln(out, line)
				}
			}
			_, _ = fmt.Fprintf(out, "next: `authama ssh vm sync` pins %s's host key and installs the key in the guest if it lacks it; then `ssh %s`\n", alias, alias)
			if !res.Moved {
				_, _ = fmt.Fprintf(out, "the original ~/.ssh/%s is untouched; remove it yourself once `ssh %s` works\n", key, alias)
			}
			return nil
		},
	}
	f := c.Flags()
	f.StringVar(&hv, "hypervisor", "", "the VM's hypervisor: "+strings.Join(manifest.KnownHypervisors(), ", "))
	f.StringVar(&vmName, "vm", "", "the VM's name as the hypervisor lists it (see 'authama ssh vm list')")
	f.StringVarP(&user, "user", "u", "", "guest user")
	f.StringVar(&keyName, "key-name", "", "the adopted key's name in the profile (default: the file's name)")
	f.BoolVar(&move, "move", false, "remove the originals from ~/.ssh once the copy is verified identical")
	f.BoolVar(&docker, "docker-context", false, "also create Docker context ssh://<alias>")
	f.BoolVarP(&dryRun, "dry-run", "n", false, "show what would happen and change nothing")
	f.BoolVarP(&yes, "yes", "y", false, "adopt without confirming or asking (implied when there is no terminal)")
	return c
}

func argAt(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

func hasVM(vms []hypervisor.VM, name string) bool {
	for _, v := range vms {
		if v.Matches(name) {
			return true
		}
	}
	return false
}

// checkAdopt validates an adoption against the manifest as it is now.
func checkAdopt(p paths.Paths, key, profile, alias, hv, user string) error {
	return change.CheckAdopt(p, key, profile, alias, hv, user)
}
