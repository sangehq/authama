package web

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/simtabi/authama/src/internal/services/bundler"
	"github.com/simtabi/authama/src/internal/services/deployer"
	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/rotator"
)

// Every job's page draws its output as a summary in one shape: these build
// the rows for the jobs that work on one key, one bundle or a set of hosts -
// deploy, roll back, restore and pin - as rotation, revoke and delete build
// theirs.

// summaryOf is a job's summary of keys: its command, how long it has run, and
// the command that runs what failed again.
type summaryOf struct {
	j                      *jobs.Job
	verb, rerun, untouched string
	began                  time.Time
}

func newSummaryOf(j *jobs.Job, verb, rerun, untouched string) summaryOf {
	return summaryOf{j: j, verb: verb, rerun: rerun, untouched: untouched, began: time.Now()}
}

// show draws keys as the job's summary.
func (s summaryOf) show(keys []rotator.SummaryKey) {
	s.j.SetSummary(rotator.Summary{Keys: keys, Elapsed: time.Since(s.began), Command: jobLine(s.j), Verb: s.verb,
		RerunCommand: s.rerun, UntouchedText: s.untouched})
}

// pending draws one row under way: the key, bundle or host the job is on.
func (s summaryOf) pending(ref, now string) { s.show(rotator.Pending([]string{ref}, now)) }

// deployRows is one row per host a key was deployed to: deployed and logged
// in with, deployed but not proved, or not deployed and why.
func deployRows(profile, keyName string, rep deployer.DeployReport) []rotator.SummaryKey {
	var out []rotator.SummaryKey
	for _, rec := range rep.Records {
		k := rotator.SummaryKey{Ref: profile + "/" + rec.Target, Key: keyName, Via: rec.Provider, Result: "deployed", Kind: rotator.KindLogsIn}
		switch {
		case rec.Error:
			k.Result, k.Severity, k.Kind, k.Note = "not deployed", 2, rotator.Classify(rec.Detail), rec.Detail
		case !rec.Verified:
			k.Severity, k.Kind = 1, rotator.KindUnverified
			k.Note = "the key is in place, but no login proved it yet: log in once to check, or deploy it again"
			if rec.Detail != "" {
				k.Note = rec.Detail
			}
		}
		out = append(out, k)
	}
	return out
}

// rollbackRows is one row per host a rollback put the old key back on.
func rollbackRows(profile, keyName string, rep rotator.RotateReport) []rotator.SummaryKey {
	var out []rotator.SummaryKey
	for _, t := range rep.Targets {
		k := rotator.SummaryKey{Ref: profile + "/" + t.Alias, Key: keyName, Via: t.Provider, Result: "rolled back", Kind: rotator.KindLogsIn}
		switch {
		case !t.Deployed:
			k.Result, k.Severity, k.Kind = "not rolled back", 2, rotator.KindFailed
			k.Note = "the old key was not put back here"
			if rep.Message != "" {
				k.Note = rep.Message
			}
		case !t.Verified:
			k.Severity, k.Kind, k.Note = 1, rotator.KindUnverified, "the old key is back in place, but no login proved it yet: log in once to check"
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		k := rotator.SummaryKey{Ref: profile + "/" + keyName, Result: "rolled back", Kind: rotator.KindLogsIn}
		if !rep.Committed {
			k.Result, k.Severity, k.Kind, k.Note = "not rolled back", 2, rotator.Classify(rep.Message), rep.Message
		}
		out = append(out, k)
	}
	return out
}

// restoreRows is one row per key a bundle laid back, with its fingerprint,
// and one per file it left to be rendered from the manifest instead.
func restoreRows(bundle string, res bundler.RestoreResult) []rotator.SummaryKey {
	owner := bundleOwner(bundle)
	var out []rotator.SummaryKey
	for _, fp := range res.Fingerprints {
		out = append(out, rotator.SummaryKey{Ref: owner + "/" + fp.Name, Key: fp.Fingerprint, Via: "age", Result: "restored"})
	}
	for _, sk := range res.Skipped {
		out = append(out, rotator.SummaryKey{Ref: owner + "/" + filepath.Base(sk), Key: sk, Via: "manifest", Result: "re-rendered",
			Note: "rendered from the manifest, not taken from the bundle"})
	}
	if len(out) == 0 {
		out = append(out, rotator.SummaryKey{Ref: owner + "/files", Key: fmt.Sprintf("%d file(s)", len(res.Restored)), Via: "age", Result: "restored"})
	}
	return out
}

// bundleOwner is a bundle's name as a tree's owner: authama-20261111-101500.
func bundleOwner(bundle string) string {
	name := filepath.Base(bundle)
	for _, ext := range []string{".age", ".tar.gz", ".tgz", ".tar"} {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			name = name[:len(name)-len(ext)]
		}
	}
	return name
}

// pinRows is one row per host a pin scanned: pinned, already trusted, no
// keys, or unreachable.
func pinRows(results []knownhosts.HostPinResult) []rotator.SummaryKey {
	var out []rotator.SummaryKey
	for _, r := range results {
		owner := r.Profile
		if owner == "" {
			owner = "global"
		}
		k := rotator.SummaryKey{Ref: owner + "/" + r.Alias, Key: fmt.Sprintf("%s:%d", r.Hostname, r.Port), Via: "ssh-keyscan", Result: "pinned"}
		switch r.Status {
		case "already-trusted":
			k.Result, k.Kind = "trusted", rotator.KindTrusted
		case "unreachable":
			k.Result, k.Severity, k.Kind = "not pinned", 2, rotator.KindUnreachable
			k.Note = fmt.Sprintf("%s:%d did not answer: pin it later, once it is reachable (Pin on the host's page)", r.Hostname, r.Port)
		case "no-keys":
			k.Result, k.Severity, k.Kind = "not pinned", 1, rotator.KindNoHostKeys
			k.Note = "the host answered with no keys to pin: check it is an ssh server on that port"
		}
		out = append(out, k)
	}
	return out
}
