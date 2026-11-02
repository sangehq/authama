package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/doctor"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
)

// newDoctorCmd is the native (engine-free) doctor verb: diagnose deps, perms,
// agent, known_hosts, and manifest-vs-disk drift/hygiene. Exits non-zero when the
// SSH report is not clean, matching v1; the token-file check warns, and fails
// only under --strict.
func newDoctorCmd() *cobra.Command {
	var fix, jsonOut, strict bool
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
			if fix {
				// --fix changes modes in the config home and ~/.ssh, so it is a
				// mutation like any other: serialized, and on the record.
				lockForMutation(p)
				for _, change := range svc.FixPerms() {
					if !jsonOut {
						_, _ = fmt.Fprintln(out, "fixed perms:", change)
					}
				}
			}
			rep := svc.Run(strict)
			// The token files (auth.json, .npmrc, docker's config.json, the git
			// credential store, .netrc) hold secrets, so the doctor checks them
			// for owner-only perms too; --fix tightens them.
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
				if tokFailed && !strict {
					_, _ = fmt.Fprintln(out, "note: a token file others can read is a warning here (doctor --fix tightens it); "+
						"--strict makes it fail, as it does in CI.")
				}
			}
			// The token check is new in 4.0. A machine whose doctor passed on
			// 3.x must still pass on 4.0, so a loose token file warns, and fails
			// only under --strict, the CI mode.
			if !rep.OK() || tokFailed && strict {
				return errNotClean
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "auto-fix perms first")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable output (scripting)")
	cmd.Flags().BoolVar(&strict, "strict", false,
		"for CI: treat every dangling-key state as a failure, not only the blocking ones, and fail on a token file others can read")
	return cmd
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
		view.Findings = append(view.Findings, finding(f))
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
