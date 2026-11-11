package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/services/vmsync"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// newDoctorCmd is the native (engine-free) doctor verb: diagnose deps, perms,
// agent, known_hosts, and manifest-vs-disk drift/hygiene. Exits non-zero when the
// SSH report is not clean, matching v1; the token-file check warns, and fails
// only under --strict.
func newDoctorCmd() *cobra.Command {
	var fix, jsonOut, strict, dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose deps, perms, agent, known_hosts, drift, dangling keys, and token-file perms",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			// A missing/invalid manifest is non-fatal: drift checks are skipped.
			m, _ := manifest.Load(p.Manifest())
			svc := doctor.New(p, m, platform.EmitUseKeychain())

			out := c.OutOrStdout()
			if dryRun && !fix {
				return fmt.Errorf("--dry-run goes with --fix: it shows what --fix would do")
			}
			if fix && dryRun {
				return doctorFixDryRun(c, p, m, svc)
			}
			if fix {
				// --fix changes modes in the config home and ~/.ssh, so it is a
				// mutation like any other: serialized, and on the record. Its
				// upkeep - vm sync, stale pins - is worked out first, asked
				// about without the lock, then worked out again under it.
				up, err := planUpkeep(p, m)
				if err != nil {
					return err
				}
				agreed := true
				if !up.empty() && !jsonOut {
					summary := "doctor --fix will also:\n  - " + strings.Join(up.lines(), "\n  - ")
					if err := confirmChange(c, summary, yes); err != nil {
						if !errors.Is(err, errAborted) {
							return err
						}
						agreed = false
						_, _ = fmt.Fprintln(out, "skipped the VM sync and the stale pins; fixing perms only")
					}
				}
				lockForMutation(p)
				for _, change := range svc.FixPerms() {
					if !jsonOut {
						_, _ = fmt.Fprintln(out, "fixed perms:", change)
					}
				}
				if agreed && !up.empty() {
					w := out
					if jsonOut {
						w = io.Discard
					}
					if err := runUpkeep(p, up, w); err != nil {
						_, _ = fmt.Fprintln(c.ErrOrStderr(), "doctor --fix:", err)
					}
				}
				m, _ = manifest.Load(p.Manifest())
				svc = doctor.New(p, m, platform.EmitUseKeychain())
			}
			rep := svc.Run(strict)
			// The token files (auth.json, .npmrc, docker's config.json, the git
			// credential store, .netrc) hold secrets, so the doctor checks them
			// for owner-only perms too; --fix tightens them.
			setLines, setFailed := settingsSummary(p)
			tokEnv := tokensvc.NewEnv(p, tokenGetenv, tokenLookup, "")
			findings, tokFailed, tokFixed := tokEnv.HomeFiles(fix)
			if jsonOut {
				b, err := rep.JSON()
				if err != nil {
					return err
				}
				if b, err = withTokens(b, findings, tokFailed); err != nil {
					return err
				}
				_, _ = fmt.Fprintln(out, string(b))
			} else {
				for _, f := range tokFixed {
					_, _ = fmt.Fprintln(out, "fixed perms:", f)
				}
				_, _ = fmt.Fprintln(out, rep.Format())
				writeDoctorTokens(out, findings)
				_, _ = fmt.Fprintln(out, "\n=== settings ===")
				for _, l := range setLines {
					_, _ = fmt.Fprintln(out, l)
				}
				if tokFailed && !strict {
					_, _ = fmt.Fprintln(out, "note: a token file others can read is a warning here (doctor --fix tightens it); "+
						"--strict makes it fail, as it does in CI.")
				}
			}
			// The token check is new in 4.0. A machine whose doctor passed on
			// 3.x must still pass on 4.0, so a loose token file warns, and fails
			// only under --strict, the CI mode.
			if !rep.OK() || tokFailed && strict || setFailed {
				return errNotClean
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "fix first: perms, stale authama pins, and a VM sync (backs up before removing)")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "with --fix: show what it would do, change nothing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "with --fix: do the upkeep without confirming (implied when there is no terminal)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output (scripting)")
	cmd.Flags().BoolVar(&strict, "strict", false,
		"for CI: treat every dangling-key state as a failure, not only the blocking ones, and fail on a token file others can read")
	return cmd
}

// upkeep is what doctor --fix does beyond perms: the VM sync, and removing
// tagged pins no manifest host uses.
type upkeep struct {
	sync      *vmsync.Syncer
	plan      vmsync.Plan
	stalePins int
}

func (u upkeep) empty() bool { return u.plan.Empty() && u.stalePins == 0 }

func (u upkeep) lines() []string {
	out := u.plan.Lines()
	if u.stalePins > 0 {
		out = append(out, fmt.Sprintf("remove %d stale authama pin(s) from known_hosts (hosts no longer in the manifest)", u.stalePins))
	}
	return out
}

func planUpkeep(p paths.Paths, m *manifest.Manifest) (upkeep, error) {
	var u upkeep
	u.sync = newSyncer(p, vmConfig(p), vmsync.DefaultOptions(nil))
	plan, err := u.sync.Plan()
	if err != nil && m != nil {
		return u, err
	}
	u.plan = plan
	if m != nil {
		if c, err := knownhosts.New(p.SSHDir).PruneCandidates(m); err == nil {
			u.stalePins = len(c)
		}
	}
	return u, nil
}

func runUpkeep(p paths.Paths, up upkeep, w io.Writer) error {
	// What was agreed to must still be what would happen.
	again, err := up.sync.Plan()
	if err != nil {
		return err
	}
	if strings.Join(again.Lines(), "\n") != strings.Join(up.plan.Lines(), "\n") {
		return errChanged
	}
	var errs []error
	if !again.Empty() {
		if err := runVMSync(w, p, up.sync, again, false); err != nil {
			errs = append(errs, err)
		}
	}
	if m, err := manifest.Load(p.Manifest()); err == nil {
		if cands, _ := knownhosts.New(p.SSHDir).PruneCandidates(m); len(cands) > 0 {
			snapshotBeforeMutation(p)
			n, err := knownhosts.New(p.SSHDir).Prune(m)
			if err != nil {
				errs = append(errs, err)
			} else {
				_, _ = fmt.Fprintf(w, "done: removed %d stale authama pin(s) from known_hosts\n", n)
			}
		}
	}
	return errors.Join(errs...)
}

func doctorFixDryRun(c *cobra.Command, p paths.Paths, m *manifest.Manifest, svc *doctor.Service) error {
	out := c.OutOrStdout()
	rep := svc.Run(false)
	_, _ = fmt.Fprintln(out, "doctor --fix would:")
	n := 0
	for _, issue := range rep.PermIssues {
		_, _ = fmt.Fprintln(out, "  - fix the mode of "+issue)
		n++
	}
	up, err := planUpkeep(p, m)
	if err != nil {
		return err
	}
	for _, l := range up.lines() {
		_, _ = fmt.Fprintln(out, "  - "+l)
		n++
	}
	if n == 0 {
		_, _ = fmt.Fprintln(out, "  nothing: modes, pins and VM hosts are all in order")
	}
	for _, note := range up.plan.Notes {
		_, _ = fmt.Fprintln(out, "note: "+note)
	}
	_, _ = fmt.Fprintln(out, "dry run: nothing changed")
	return nil
}

func writeDoctorTokens(w io.Writer, fs []tokensvc.Finding) {
	_, _ = fmt.Fprintln(w, "\n=== auth tokens ===")
	if len(fs) == 0 {
		_, _ = fmt.Fprintln(w, "no token files installed")
		return
	}
	printFindings(w, fs)
}

// withTokens adds a "tokens" member to the doctor's JSON object, keeping the
// SSH report's keys and their order as they were: {"ok": ..., "findings": [...]}.
func withTokens(report []byte, fs []tokensvc.Finding, failed bool) ([]byte, error) {
	type finding struct {
		Level  string `json:"level"`
		Target string `json:"target"`
		Text   string `json:"text"`
	}
	view := struct {
		OK       bool      `json:"ok"`
		Findings []finding `json:"findings"`
	}{OK: !failed, Findings: []finding{}}
	for _, f := range fs {
		view.Findings = append(view.Findings, finding{Level: f.Level, Target: f.Target, Text: f.Text})
	}
	tok, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimRight(report, " \t\r\n")
	if len(trimmed) < 2 || trimmed[len(trimmed)-1] != '}' {
		return nil, fmt.Errorf("doctor: the report is not a JSON object")
	}
	var compact bytes.Buffer
	spliced := append(append(append([]byte{}, trimmed[:len(trimmed)-1]...), []byte(`,"tokens":`)...), tok...)
	spliced = append(spliced, '}')
	if !bytes.Contains(report, []byte("\n")) {
		if err := json.Compact(&compact, spliced); err != nil {
			return nil, err
		}
		return compact.Bytes(), nil
	}
	if err := json.Indent(&compact, spliced, "", "  "); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}
