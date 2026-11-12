package web

import (
	"fmt"
	"github.com/simtabi/authama/src/internal/services/rotator"
	"net/url"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/bundler"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/lifecycle"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// keySelection reads which keys a bulk job acts on: "all", or any mix of
// "sel" values naming a key (profile/key), a profile, or a category
// ("category:<name>", every profile filed under it). The older form's single
// "profile" still works. It returns the selection, the command's arguments
// and the history subjects.
func keySelection(f url.Values, p paths.Paths) (keysvc.Selection, []string, []history.Subject, error) {
	if checked(f, "all") {
		return keysvc.Selection{All: true}, []string{"--all"}, nil, nil
	}
	var picked []string
	if f.Get("profile") != "" {
		prof, err := name(f, "profile")
		if err != nil {
			return keysvc.Selection{}, nil, nil, err
		}
		picked = append(picked, prof)
	}
	var categories []string
	for _, v := range f["sel"] {
		v = strings.TrimSpace(v)
		if c, ok := strings.CutPrefix(v, "category:"); ok {
			if !nameRe.MatchString(c) {
				return keysvc.Selection{}, nil, nil, badForm("%q is not a category authama accepts", c)
			}
			categories = append(categories, c)
			continue
		}
		prof, key, isKey := strings.Cut(v, "/")
		if !nameRe.MatchString(prof) || (isKey && !nameRe.MatchString(key)) {
			return keysvc.Selection{}, nil, nil, badForm("%q is not a key or profile authama accepts", v)
		}
		picked = append(picked, v)
	}
	if len(categories) > 0 {
		m, err := manifest.Load(p.Manifest())
		if err != nil {
			return keysvc.Selection{}, nil, nil, err
		}
		for _, c := range categories {
			found := false
			for _, prof := range m.ProfileNames() {
				if m.Profiles[prof].Category == c {
					picked, found = append(picked, prof), true
				}
			}
			if !found {
				return keysvc.Selection{}, nil, nil, badForm("no profile is filed under the category %q", c)
			}
		}
	}
	if len(picked) == 0 {
		return keysvc.Selection{}, nil, nil, badForm("choose at least one key or profile, or every key")
	}
	picked = dedupe(picked)
	var subjects []history.Subject
	for _, v := range picked {
		if prof, key, ok := strings.Cut(v, "/"); ok {
			subjects = append(subjects, history.Subject{Kind: history.KindProfile, Name: prof}, history.Subject{Kind: history.KindKey, Name: key})
		} else {
			subjects = append(subjects, history.Subject{Kind: history.KindProfile, Name: v})
		}
	}
	return keysvc.Selection{Selectors: picked}, append([]string(nil), picked...), subjects, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v], out = true, append(out, v)
		}
	}
	return out
}

// chosenRows resolves a selection to its rows.
func chosenRows(p paths.Paths, sel keysvc.Selection) ([]keysvc.Row, error) {
	m, inv, err := loadState(p)
	if err != nil {
		return nil, err
	}
	rows, err := keysvc.New(m, inv, p.SSHDir).Select(sel)
	if err != nil {
		return nil, badForm("%s", err.Error())
	}
	return rows, nil
}

// revokeKeysSpec revokes each chosen key that has been recorded (minted and
// fingerprinted): a key never minted has nothing to revoke.
func revokeKeysSpec(f url.Values, h home) (jobSpec, error) {
	sel, args, subjects, err := keySelection(f, h.p)
	if err != nil {
		return jobSpec{}, err
	}
	reason, err := plain(f, "reason")
	if err != nil {
		return jobSpec{}, err
	}
	rows, err := chosenRows(h.p, sel)
	if err != nil {
		return jobSpec{}, err
	}
	var refs []manifest.KeyRef
	var names []string
	for _, r := range rows {
		if r.Recorded && r.Fingerprint != "" {
			refs, names = append(refs, r.Ref), append(names, r.Ref.String())
		}
	}
	if len(refs) == 0 {
		return jobSpec{}, badForm("none of the chosen keys has been minted, so there is nothing to revoke")
	}
	args = append(args, "--yes")
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	return jobSpec{
		scope: mutation.Scope{Command: "key revoke", Args: args, Source: history.SourceWeb, Subjects: subjects},
		what: []string{
			fmt.Sprintf("revokes %d key(s), one at a time, in the order listed", len(names)),
			"asks every target each key is deployed to, through its host's provider, to remove it",
			"names each target a key could not be removed from, for you to remove it there by hand",
			"records each key as revoked, so deploy and rollback refuse it; each key's checklist then lists what is left",
			"a key that fails is reported, and the rest go on",
		},
		run:   func(j *jobs.Job) error { return revokeKeysJob(j, h.p, refs, reason) },
		items: names,
	}, nil
}

func revokeKeysJob(j *jobs.Job, p paths.Paths, refs []manifest.KeyRef, reason string) error {
	mutation.LockForMutation(p)
	if err := j.Step(); err != nil {
		return err
	}
	var failed []string
	names := refStrings(refs)
	j.Plan(names)
	live := newLiveSummary(j, "revoke", "revoked", "not revoked",
		"Keys marked not revoked are as they were: they still log in where they were deployed.")
	defer live.done()
	for i, ref := range refs {
		if err := j.Step(); err != nil {
			return err
		}
		j.Progress(i, len(refs), "revoking "+ref.String())
		live.show(names[i:], "revoking")
		res, err := editor.New(p).Unattended(j).RevokeKey(ref, reason)
		if err != nil {
			failed = append(failed, ref.String())
			j.Failed(ref.String())
			live.fail(ref.String(), err.Error())
			_, _ = fmt.Fprintf(j, "%s: NOT revoked: %v\n", ref, err)
			continue
		}
		if len(res.NotRemoved) > 0 {
			live.partly(ref.String(), "recorded as revoked, but not removed from "+strings.Join(res.NotRemoved, ", ")+": remove it there by hand")
		} else {
			live.ok(ref.String())
		}
		_, _ = fmt.Fprintf(j, "revoked %s (%s)\n", res.Key, res.Fingerprint)
		if len(res.Removed) > 0 {
			_, _ = fmt.Fprintf(j, "  removed from: %s\n", strings.Join(res.Removed, ", "))
		}
		if len(res.NotRemoved) > 0 {
			failed = append(failed, ref.String())
			j.Failed(ref.String())
			_, _ = fmt.Fprintf(j, "  NOT removed from (remove it there by hand): %s\n", strings.Join(res.NotRemoved, ", "))
		}
	}
	j.Progress(len(refs), len(refs), "")
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d key(s) need more: %s; see above, and each key's checklist", len(failed), len(refs), strings.Join(failed, ", "))
	}
	return nil
}

// deleteKeysSpec deletes each chosen key no host uses: a key a host still
// uses would leave the config pointing at a file that is gone, so it is left
// alone and named.
func deleteKeysSpec(f url.Values, h home) (jobSpec, error) {
	sel, args, subjects, err := keySelection(f, h.p)
	if err != nil {
		return jobSpec{}, err
	}
	opt := lifecycle.Options{Revoke: checked(f, "revoke"), Purge: checked(f, "purge")}
	rows, err := chosenRows(h.p, sel)
	if err != nil {
		return jobSpec{}, err
	}
	var refs []manifest.KeyRef
	var names, wired []string
	for _, r := range rows {
		if r.Wired() {
			wired = append(wired, r.Ref.String())
			continue
		}
		refs, names = append(refs, r.Ref), append(names, r.Ref.String())
	}
	if len(refs) == 0 {
		return jobSpec{}, badForm("every chosen key is used by a host; delete or re-point those hosts first (%s)", strings.Join(wired, ", "))
	}
	if opt.Revoke {
		args = append(args, "--revoke")
	}
	if opt.Purge {
		args = append(args, "--purge")
	}
	what := []string{
		fmt.Sprintf("deletes %d key(s)' declaration and inventory record, one at a time, in the order listed", len(names)),
	}
	if opt.Revoke {
		what = append(what, "asks each target the keys are deployed to to remove them first")
	}
	if opt.Purge {
		what = append(what, "deletes the key files too, after writing an encrypted backup")
	} else {
		what = append(what, "keeps the key files on disk")
	}
	if len(wired) > 0 {
		what = append(what, fmt.Sprintf("leaves %d key(s) a host still uses: %s", len(wired), strings.Join(wired, ", ")))
	}
	return jobSpec{
		scope: mutation.Scope{Command: "key delete", Args: append(args, "--yes"), Source: history.SourceWeb, Subjects: subjects},
		what:  what,
		run:   func(j *jobs.Job) error { return deleteKeysJob(j, h.p, h.retain, refs, opt) },
		items: names,
	}, nil
}

func deleteKeysJob(j *jobs.Job, p paths.Paths, retain int, refs []manifest.KeyRef, opt lifecycle.Options) error {
	mutation.LockForMutation(p)
	// As `ssh key delete --purge` does: the files go only after an encrypted
	// backup, and ~/.ssh is snapshotted before anything changes.
	if opt.Purge {
		if err := bundler.BackupBeforeDestroying(p, j, false); err != nil {
			return err
		}
	}
	if retain < 1 {
		retain = 10
	}
	mutation.SnapshotBeforeMutation(p, retain)
	if err := j.Step(); err != nil {
		return err
	}
	var failed []string
	names := refStrings(refs)
	j.Plan(names)
	live := newLiveSummary(j, "delete", "deleted", "not deleted", "Keys marked not deleted are as they were.")
	defer live.done()
	for i, ref := range refs {
		if err := j.Step(); err != nil {
			return err
		}
		j.Progress(i, len(refs), "deleting "+ref.String())
		live.show(names[i:], "deleting")
		res, err := lifecycle.New(p, platform.EmitUseKeychain()).DeleteKey(ref, opt)
		if err != nil {
			failed = append(failed, ref.String())
			j.Failed(ref.String())
			live.fail(ref.String(), err.Error())
			_, _ = fmt.Fprintf(j, "%s: NOT deleted: %v\n", ref, err)
			continue
		}
		if len(res.NotRevoked) > 0 {
			live.partly(ref.String(), "deleted here, but not removed from "+strings.Join(res.NotRevoked, ", ")+": remove it there by hand")
		} else {
			live.ok(ref.String())
		}
		_, _ = fmt.Fprintf(j, "deleted %s\n", res.Removed)
		if len(res.NotRevoked) > 0 {
			_, _ = fmt.Fprintf(j, "  NOT revoked from (remove it there by hand): %s\n", strings.Join(res.NotRevoked, ", "))
		}
	}
	j.Progress(len(refs), len(refs), "")
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d key(s) were not deleted: %s; see above", len(failed), len(refs), strings.Join(failed, ", "))
	}
	return nil
}

// refStrings is each key as profile/key, for a job's list of items.
func refStrings(refs []manifest.KeyRef) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.String()
	}
	return out
}

// liveSummary keeps a bulk key job's summary current as it goes, in the shape
// a rotation's takes: each key finished with its result, the one under way,
// and the rest waiting.
type liveSummary struct {
	j               *jobs.Job
	verb            string
	okWord, badWord string
	untouched, line string
	began           time.Time
	keys            []rotator.SummaryKey
}

func newLiveSummary(j *jobs.Job, verb, okWord, badWord, untouched string) *liveSummary {
	return &liveSummary{j: j, verb: verb, okWord: okWord, badWord: badWord, untouched: untouched,
		line: jobLine(j), began: time.Now()}
}

func (l *liveSummary) ok(ref string) {
	l.keys = append(l.keys, rotator.SummaryKey{Ref: ref, Result: l.okWord})
}

func (l *liveSummary) partly(ref, note string) {
	l.keys = append(l.keys, rotator.SummaryKey{Ref: ref, Result: "partly " + l.okWord, Severity: 1,
		Kind: rotator.KindNotRemoved, Note: note})
}

func (l *liveSummary) fail(ref, note string) {
	l.keys = append(l.keys, rotator.SummaryKey{Ref: ref, Result: l.badWord, Severity: 2, Kind: rotator.KindFailed, Note: note})
}

// show draws the summary with rest still to do, the first of them under way.
func (l *liveSummary) show(rest []string, now string) {
	sum := rotator.Summary{Keys: append(append([]rotator.SummaryKey(nil), l.keys...), rotator.Pending(rest, now)...),
		Elapsed: time.Since(l.began), Command: l.line, Verb: l.verb, UntouchedText: l.untouched}
	if failed := sum.Failed(); len(failed) > 0 {
		sum.RerunCommand = "authama ssh key " + l.verb + " " + strings.Join(failed, " ")
	}
	l.j.SetSummary(sum)
}

func (l *liveSummary) done() { l.show(nil, "") }
