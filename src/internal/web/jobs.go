package web

import (
	"errors"
	"fmt"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/reconciler"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/bundler"
	"github.com/simtabi/authama/src/internal/services/deployer"
	"github.com/simtabi/authama/src/internal/services/editor"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/services/keystore"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/rotator"
	"github.com/simtabi/authama/src/internal/util/envnames"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// A job is a change that reaches other machines - deploy, rotate, rollback -
// so no copy can preview it. POST /jobs/review says what it will do and what
// it is the same as; POST /jobs starts it, and its page streams the output
// from GET /jobs/{id}/events. One job runs at a time, under the lock for as
// long as it runs. A job can ask a question (rotate's "pasted the key?"),
// answered at POST /jobs/{id}/answer, and be cancelled between its steps at
// POST /jobs/{id}/cancel. Nothing a job runs can prompt: ssh and ssh-copy-id
// run with stdin closed and BatchMode=yes (W12), and passphrases are never
// taken.

// jobFields are the form fields each job reads.
var jobFields = map[string][]string{
	"deploy":   {"key", "target"},
	"rotate":   {"key", "allow_unverified", "no_backup", "check", "force"},
	"rollback": {"key"},
	// revoke removes a key from every target and records it as revoked.
	"revoke": {"key", "reason"},
	// rotate-keys rotates the chosen keys (sel: keys, profiles, categories),
	// a profile's keys, or every key, one after another.
	"rotate-keys": {"sel", "profile", "all", "allow_unverified", "no_backup", "check", "force"},
	// revoke-keys revokes each chosen key, as `authama ssh key revoke` does.
	"revoke-keys": {"sel", "all", "reason"},
	// delete-keys deletes each chosen key, as `authama ssh key delete` does.
	"delete-keys": {"sel", "all", "revoke", "purge"},
	// bundle-restore lays a bundle from the state home's dist/ back.
	"bundle-restore": {"bundle"},
	// knownhosts-init scans a profile's hosts, or every host, and pins what
	// answers (trust on first use), as `ssh knownhosts init` does.
	"knownhosts-init": {"profile", "all", "force"},
	// knownhosts-pin scans one manifest host and pins its keys once the
	// person has seen their fingerprints, as `ssh knownhosts pin` does.
	"knownhosts-pin": {"host"},
}

// readsOnlyHostKeys is the jobs that only ask a host for its public keys and
// write the local known_hosts. Nothing on a server changes, so a dev sandbox
// runs them, as the CLI does.
var readsOnlyHostKeys = map[string]bool{"knownhosts-init": true, "knownhosts-pin": true}

// bundleRe is what a bundle the web restores is named: authama-<stamp>.age as
// bundler writes it, or ssh-manager-<stamp>.age from a release before 0.1.1
// (bundler.LegacyBundlePrefix), which is still restorable.
var bundleRe = regexp.MustCompile(`^(authama|ssh-manager)-[0-9]{8}-[0-9]{6}(-[0-9]+)?\.age$`)

// jobSpec is a job ready to start: what it is, what it will do, and the steps.
type jobSpec struct {
	scope mutation.Scope
	what  []string
	run   func(j *jobs.Job) error
	// title names the job for people, "Rotate 10 keys"; items are what it
	// works through, one at a time, listed on the review; backup says it
	// writes an encrypted backup first, so it needs an age recipient; and
	// destroy says it cannot be undone from authama.
	title   string
	items   []string
	backup  bool
	destroy bool
	check   bool // a dry run: it changes nothing
}

// keyRef reads a "profile/key" field.
func keyRef(f url.Values) (manifest.KeyRef, error) {
	profile, key, ok := strings.Cut(strings.TrimSpace(f.Get("key")), "/")
	if !ok || !nameRe.MatchString(profile) || !nameRe.MatchString(key) {
		return manifest.KeyRef{}, badForm("key must be profile/key, each a name authama accepts")
	}
	return manifest.KeyRef{Profile: profile, KeyName: key}, nil
}

// buildJob turns a form into the job it asks for.
func buildJob(f url.Values, h home) (jobSpec, error) {
	action := f.Get("action")
	if _, ok := jobFields[action]; !ok {
		return jobSpec{}, badForm("unknown job %q", action)
	}
	// Everything but a bundle restore reaches real servers, which a sandbox
	// cannot contain: refused here, as the CLI refuses it.
	// Deleting keys without revoking them reaches no server.
	localOnly := action == "delete-keys" && !checked(f, "revoke")
	if action != "bundle-restore" && !readsOnlyHostKeys[action] && !localOnly {
		verb := action
		switch verb {
		case "rotate-keys":
			verb = "rotate"
		case "revoke-keys":
			verb = "revoke"
		case "delete-keys":
			verb = "delete"
		}
		if err := h.p.RefuseOutsideSandbox(verb); err != nil {
			return jobSpec{}, badForm("%s", err.Error())
		}
	}
	retain := h.retain
	if retain < 1 {
		retain = 10
	}
	switch action {
	case "rotate-keys":
		return rotateKeysSpec(f, h, retain)
	case "revoke-keys":
		return revokeKeysSpec(f, h)
	case "delete-keys":
		return deleteKeysSpec(f, h)
	case "bundle-restore":
		return bundleRestoreSpec(f, h, retain)
	case "knownhosts-init":
		return knownhostsInitSpec(f, h, retain)
	case "knownhosts-pin":
		return knownhostsPinSpec(f, h, retain)
	}
	ref, err := keyRef(f)
	if err != nil {
		return jobSpec{}, err
	}
	key := ref.String()
	subjects := []history.Subject{{Kind: history.KindProfile, Name: ref.Profile}, {Kind: history.KindKey, Name: ref.KeyName}}
	switch action {
	case "deploy":
		target, err := optionalName(f, "target")
		if err != nil {
			return jobSpec{}, err
		}
		args := []string{key}
		where := "every host that uses it"
		if target != "" {
			args, where = append(args, target), target
		}
		return jobSpec{
			scope: mutation.Scope{Command: "deploy", Args: append(args, "--yes"), Source: history.SourceWeb, Subjects: subjects},
			what: []string{
				"installs the public half of " + key + " on " + where + ", and records where it went",
				"snapshots ~/.ssh first",
				"cannot answer a password prompt: a target that needs a password fails, and is deployed from a terminal",
			},
			run: func(j *jobs.Job) error { return deployJob(j, h.p, retain, key, target) },
		}, nil
	case "rotate":
		if checked(f, "check") {
			return rotateCheckSpec([]string{key}, []string{key, "--check"}, subjects, h), nil
		}
		allow, skip, force := checked(f, "allow_unverified"), checked(f, "no_backup"), checked(f, "force")
		args := []string{key, "--yes"}
		if allow {
			args = append(args, "--allow-unverified")
		}
		if skip {
			args = append(args, "--no-key-backup")
		}
		if force {
			args = append(args, "--force")
		}
		return jobSpec{
			scope: mutation.Scope{Command: "rotate", Args: args, Source: history.SourceWeb, Subjects: subjects},
			what: append([]string{backupStep(skip)},
				"mints a new "+key+", deploys it to every host that uses the key, and proves each login",
				"moves the current key to old/, replacing any earlier predecessor, once every target is done",
				"asks you to paste the new public key where a target takes it by hand, and waits",
				forceStep(force)),
			run:   func(j *jobs.Job) error { return rotateJob(j, h.p, retain, key, allow, skip, force) },
			items: []string{key},
		}, nil
	case "revoke":
		reason, err := plain(f, "reason")
		if err != nil {
			return jobSpec{}, err
		}
		args := []string{key, "--yes"}
		if reason != "" {
			args = append(args, "--reason", reason)
		}
		return jobSpec{
			scope: mutation.Scope{Command: "key revoke", Args: args, Source: history.SourceWeb, Subjects: subjects},
			what: []string{
				"asks every target " + key + " is deployed to, through its host's provider, to remove it",
				"names each target it could not be removed from, for you to remove it there by hand",
				"records the key as revoked, so deploy and rollback refuse it from then on",
				"keeps the key's declaration, hosts and files: mint its successor next, with Rotate or keygen --force",
			},
			run: func(j *jobs.Job) error { return revokeJob(j, h.p, ref, reason) },
		}, nil
	default: // rollback
		return jobSpec{
			scope: mutation.Scope{Command: "rollback", Args: []string{key, "--yes"}, Source: history.SourceWeb, Subjects: subjects},
			what: []string{
				"puts " + key + "'s old/ predecessor back, and keeps the rotated-in key in old/ so this can be undone",
				"checks every target is reachable first, and changes nothing if one is not",
				"deploys the restored key to each target, and snapshots ~/.ssh first",
			},
			run: func(j *jobs.Job) error { return rollbackJob(j, h.p, retain, key) },
		}, nil
	}
}

func loadState(p paths.Paths) (*manifest.Manifest, *inventory.Inventory, error) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return nil, nil, err
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		return nil, nil, err
	}
	return m, inv, nil
}

// deployJob is `authama ssh deploy` after its confirmation.
func deployJob(j *jobs.Job, p paths.Paths, retain int, key, target string) error {
	m, inv, err := loadState(p)
	if err != nil {
		return err
	}
	mutation.SnapshotBeforeMutation(p, retain)
	if err := j.Step(); err != nil {
		return err
	}
	profile, keyName := splitKey(m, key)
	sum := newSummaryOf(j, "deploy", "authama ssh deploy "+profile+"/"+keyName,
		"Hosts marked not deployed do not have the key: it cannot log in there yet.")
	sum.pending(profile+"/"+keyName, "deploying")
	report, err := deployer.New(p, m, inv).Unattended(j).Deploy(key, target)
	if err != nil {
		sum.show([]rotator.SummaryKey{{Ref: profile + "/" + keyName, Result: "not deployed", Severity: 2,
			Kind: rotator.Classify(err.Error()), Note: err.Error()}})
		return err
	}
	if err := inv.Save(p.Inventory()); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(j, report.Format())
	sum.show(deployRows(profile, keyName, report))
	if report.AnyError() {
		return errors.New("not every target took the key; see above")
	}
	return nil
}

// rotateJob is rotate's single-key path after its confirmation. A target that
// takes the key by hand pauses the job with the new public key to paste.
func rotateJob(j *jobs.Job, p paths.Paths, retain int, key string, allowUnverified, skipBackup, force bool) error {
	if err := p.RefuseOutsideSandbox("rotate"); err != nil {
		return err
	}
	if err := bundler.BackupBeforeDestroying(p, j, skipBackup); err != nil {
		return err
	}
	m, inv, err := loadState(p)
	if err != nil {
		return err
	}
	mutation.SnapshotBeforeMutation(p, retain)
	if err := j.Step(); err != nil {
		return err
	}
	r := rotator.New(p, m, inv).Unattended(j).WithManualStep(manualPaste(j))
	began, line := time.Now(), jobLine(j)
	res := rotator.RunResult{Ref: key, Via: r.Via(key)}
	// The summary of one key: under way, then how it went.
	show := func(replaced []string) {
		sum := rotator.SummarizeRun([]rotator.RunResult{res}, replaced, time.Since(began))
		if res.Report == nil && res.Err == nil {
			sum = rotator.Summary{Keys: rotator.Pending([]string{key}, "rotating")}
		}
		sum.Command = line
		j.SetSummary(sum)
	}
	show(nil)
	report, err := r.Rotate(key, allowUnverified, "")
	if err != nil {
		res.Err = err
		show(nil)
		return err
	}
	res.Report = &report
	if report.Committed {
		if err := inv.Save(p.Inventory()); err != nil {
			return err
		}
	}
	_, _ = fmt.Fprintln(j, report.Format())
	if !report.Committed {
		if still := replaceFailed(j, p, []string{key}, force); len(still) == 0 {
			show([]string{key})
			return nil
		}
		show(nil)
		return errors.New("the rotation did not complete; see above")
	}
	show(nil)
	return nil
}

// rollbackJob is `authama ssh rollback` after its confirmation.
func rollbackJob(j *jobs.Job, p paths.Paths, retain int, key string) error {
	m, inv, err := loadState(p)
	if err != nil {
		return err
	}
	mutation.SnapshotBeforeMutation(p, retain)
	if err := j.Step(); err != nil {
		return err
	}
	profile, keyName := splitKey(m, key)
	sum := newSummaryOf(j, "rollback", "authama ssh rollback "+profile+"/"+keyName,
		"Hosts marked not rolled back still trust the newer key.")
	sum.pending(profile+"/"+keyName, "rolling back")
	report, err := rotator.New(p, m, inv).Unattended(j).Rollback(key)
	if err != nil {
		sum.show([]rotator.SummaryKey{{Ref: profile + "/" + keyName, Result: "not rolled back", Severity: 2,
			Kind: rotator.Classify(err.Error()), Note: err.Error()}})
		return err
	}
	if err := inv.Save(p.Inventory()); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(j, report.Format())
	sum.show(rollbackRows(profile, keyName, report))
	if !report.Committed {
		return errors.New("the rollback did not complete; see above")
	}
	return nil
}

// rotateKeysSpec is `authama ssh rotate <profile> --yes` or `authama ssh rotate --all
// --yes`: every key the selection holds that a host uses, one after another.
func rotateKeysSpec(f url.Values, h home, retain int) (jobSpec, error) {
	sel, args, subjects, err := keySelection(f, h.p)
	if err != nil {
		return jobSpec{}, err
	}
	allow, skip, force := checked(f, "allow_unverified"), checked(f, "no_backup"), checked(f, "force")
	names, err := selected(h.p, sel)
	if err != nil {
		return jobSpec{}, err
	}
	if len(names) == 0 {
		return jobSpec{}, badForm("no key there is used by a host, so there is nothing to rotate")
	}
	if checked(f, "check") {
		return rotateCheckSpec(names, append(args, "--check"), subjects, h), nil
	}
	if allow {
		args = append(args, "--allow-unverified")
	}
	if skip {
		args = append(args, "--no-key-backup")
	}
	if force {
		args = append(args, "--force")
	}
	return jobSpec{
		scope: mutation.Scope{Command: "rotate", Args: append(args, "--yes"), Source: history.SourceWeb, Subjects: subjects},
		what: []string{
			backupStep(skip),
			fmt.Sprintf("rotates %d key(s), one at a time, in the order listed: each is minted, deployed and logged in with before the next", len(names)),
			"a key that fails is left as it was, and the rest go on",
			"asks you to paste a new public key wherever a target takes it by hand, and waits",
			forceStep(force),
		},
		run:   func(j *jobs.Job) error { return rotateKeysJob(j, h.p, retain, sel, allow, skip, force) },
		items: names,
	}, nil
}

// selected is the keys a selection rotates: those a host uses.
func selected(p paths.Paths, sel keysvc.Selection) ([]string, error) {
	m, inv, err := loadState(p)
	if err != nil {
		return nil, err
	}
	rows, err := keysvc.New(m, inv, p.SSHDir).Select(sel)
	if err != nil {
		return nil, badForm("%s", err.Error())
	}
	var names []string
	for _, r := range rows {
		if r.Wired() {
			names = append(names, r.Ref.String())
		}
	}
	return names, nil
}

func rotateKeysJob(j *jobs.Job, p paths.Paths, retain int, sel keysvc.Selection, allowUnverified, skipBackup, force bool) error {
	if err := p.RefuseOutsideSandbox("rotate"); err != nil {
		return err
	}
	if err := bundler.BackupBeforeDestroying(p, j, skipBackup); err != nil {
		return err
	}
	m, inv, err := loadState(p)
	if err != nil {
		return err
	}
	// Worked out again under the lock: the review was a moment ago.
	names, err := selected(p, sel)
	if err != nil {
		return err
	}
	mutation.SnapshotBeforeMutation(p, retain)
	j.Plan(names)
	r := rotator.New(p, m, inv).Unattended(j).WithManualStep(manualPaste(j))
	// RotateEach calls next once before each key, so the count it keeps is
	// the number of keys already finished.
	at := 0
	began := time.Now()
	line := jobLine(j)
	show := func(rest []string, now string, replaced []string) rotator.Summary {
		sum := rotator.SummarizeRun(r.Results(), replaced, time.Since(began))
		sum.Command = line
		sum.Keys = append(sum.Keys, rotator.Pending(rest, now)...)
		j.SetSummary(sum)
		return sum
	}
	next := func() error {
		if err := j.Step(); err != nil {
			return err
		}
		j.Progress(at, len(names), "rotating "+names[at])
		show(names[at:], "rotating", nil)
		at++
		return nil
	}
	failed, err := r.RotateEach(names, allowUnverified, "", j, func() error { return inv.Save(p.Inventory()) }, next)
	if err != nil {
		return err
	}
	before := failed
	failed = replaceFailed(j, p, failed, force)
	_ = rotator.SaveFailed(p, show(nil, "", without(before, failed)).Failed())
	for _, n := range failed {
		j.Failed(n)
	}
	j.Progress(len(names), len(names), "")
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d key(s) were not rotated, and are as they were: %s",
			len(failed), len(names), strings.Join(failed, ", "))
	}
	return nil
}

// manualPaste is rotate's pause at a target that takes the key by hand.
func manualPaste(j *jobs.Job) rotator.ManualStep {
	return func(alias, where, pubkey string) bool {
		_, _ = fmt.Fprintf(j, "%s: this target takes the key by hand. %s\n", alias, where)
		return j.Ask("Paste this key where "+alias+" takes it, then confirm: authama will log in with it to prove it.",
			strings.TrimSpace(pubkey))
	}
}

// bundleRestoreSpec is `authama ssh restore <bundle> --yes`, for a bundle in the
// state home's dist/, with the identity from AUTHAMA_AGE_IDENTITY_FILE.
// The web takes no identity, and no passphrase for one.
func bundleRestoreSpec(f url.Values, h home, retain int) (jobSpec, error) {
	b := strings.TrimSpace(f.Get("bundle"))
	if !bundleRe.MatchString(b) {
		return jobSpec{}, badForm("bundle %q is not a bundle authama writes", b)
	}
	path := filepath.Join(h.p.DistDir(), b)
	if _, err := os.Stat(path); err != nil {
		return jobSpec{}, badForm("no bundle %s in %s", b, h.p.DistDir())
	}
	return jobSpec{
		scope: mutation.Scope{Command: "restore", Args: []string{b, "--yes"}, Source: history.SourceWeb},
		what: []string{
			"decrypts " + b + " with the identity in AUTHAMA_AGE_IDENTITY_FILE, which cannot have a passphrase here",
			"lays its keys and config-home files back over what is there, snapshotting ~/.ssh first",
			"then sets every mode right and renders ~/.ssh/config",
		},
		run: func(j *jobs.Job) error { return bundleRestoreJob(j, h.p, retain, path) },
	}, nil
}

func bundleRestoreJob(j *jobs.Job, p paths.Paths, retain int, path string) error {
	ident := envnames.Get(nil, envnames.AgeIdentityFile)
	if ident == "" {
		return errors.New("AUTHAMA_AGE_IDENTITY_FILE is not set where authama serve runs; restore from a terminal " +
			"with `authama ssh restore " + filepath.Base(path) + " -i <identity>`")
	}
	mutation.SnapshotBeforeMutation(p, retain)
	if err := j.Step(); err != nil {
		return err
	}
	sum := newSummaryOf(j, "restore", "", "")
	sum.pending(bundleOwner(path)+"/keys", "restoring")
	res, err := bundler.New(p.SSHDir, p.ConfigDir, bundler.AgeCipher{}).WithKeyRoot(manifest.KeyRootAt(p.Manifest())).Restore(path, ident, keystore.New().Fingerprint)
	if err != nil {
		sum.show([]rotator.SummaryKey{{Ref: bundleOwner(path) + "/keys", Result: "not restored", Severity: 2,
			Kind: rotator.KindFailed, Note: err.Error()}})
		return err
	}
	finishErr := bundler.FinishRestore(p)
	_, _ = fmt.Fprintln(j, res.Format())
	sum.show(restoreRows(path, res))
	if finishErr != nil {
		return fmt.Errorf("keys restored, but ~/.ssh is not usable yet: %w", finishErr)
	}
	return nil
}

type jobReview struct {
	Title   string
	Command string
	What    []string
	Items   []string
	Checks  []preflight
	Blocked bool
	Destroy bool
	Remote  bool // it logs in to other machines
	// CanCheck offers a dry run of this job first; Check says it is one.
	CanCheck, Check bool
	Fields          []formField
	Busy            *jobs.View
	// Back is the page the job was chosen on, and BackLabel names it: where
	// Cancel goes.
	Back, BackLabel string
	// Options are the switches the review can be done again with; Kept are
	// the fields carried past them.
	Options []jobOption
	Kept    []formField
	// Forced is a forced rotation; Backup says an encrypted backup is written
	// first.
	Forced, Backup bool
}

// jobOption is one switch on a job's review: turning it reviews the job
// again with the field set or cleared.
type jobOption struct {
	Name, Label, Detail string
	On, Danger          bool
}

// jobOptions are the switches a job's review offers, and the fields carried
// past them unchanged.
func jobOptions(f url.Values, fields []formField) ([]jobOption, []formField) {
	var opts []jobOption
	switch f.Get("action") {
	case "rotate", "rotate-keys":
		opts = []jobOption{
			{Name: "no_backup", Label: "Skip the encrypted backup",
				Detail: "A clean rotation: no backup bundle is written. Each replaced key is still kept under old/ for a rollback."},
			{Name: "allow_unverified", Label: "Go on when a login cannot be verified",
				Detail: "Off, a key whose new login cannot be proved stops the job and keeps working as before."},
			{Name: "force", Label: "Force: replace what cannot rotate in place", Danger: true,
				Detail: "A key never deployed, or whose host cannot be reached, is replaced on this computer anyway and must be deployed next. Until then a host that trusted the old key cannot be logged in to with it; the old key stays under old/ for a rollback."},
		}
	}
	names := map[string]bool{}
	for i := range opts {
		opts[i].On = checked(f, opts[i].Name)
		names[opts[i].Name] = true
	}
	var kept []formField
	for _, fl := range fields {
		if !names[fl.Name] && fl.Name != "check" {
			kept = append(kept, fl)
		}
	}
	return opts, kept
}

// jobBack is the page a job was chosen on: a key's own page, the list it
// was picked from, Snapshots or Hosts.
func jobBack(f url.Values) (href, label string) {
	switch f.Get("action") {
	case "deploy", "rotate", "rollback", "revoke":
		if k := f.Get("key"); strings.Contains(k, "/") {
			return "/keys/" + k, k
		}
	case "bundle-restore":
		return "/snapshots", "Snapshots"
	case "knownhosts-init", "knownhosts-pin":
		return "/hosts", "Hosts"
	}
	return "/keys", "Keys"
}

func jobCommand(s mutation.Scope) string {
	return strings.TrimSpace(brand.Line(s.Command) + " " + strings.Join(s.Args, " "))
}

func carriedJob(f url.Values) []formField {
	out := []formField{{"action", f.Get("action")}}
	for _, k := range jobFields[f.Get("action")] {
		// sel holds every key chosen, so every value goes on.
		for _, v := range f[k] {
			out = append(out, formField{k, v})
		}
	}
	return out
}

// jobPages holds the job handlers, kept off pages for the same reason pages
// is kept off Server: no type grows into a facade.
type jobPages struct{ s *Server }

func (h jobPages) fail(w http.ResponseWriter, r *http.Request, err error) { pages(h).fail(w, r, err) }

func (h jobPages) parseJob(w http.ResponseWriter, r *http.Request) (jobSpec, bool) {
	if err := r.ParseForm(); err != nil {
		h.s.renderError(w, r, http.StatusBadRequest, "Unreadable form", err.Error())
		return jobSpec{}, false
	}
	spec, err := buildJob(r.PostForm, home{p: h.s.opts.Engine.Home(), retain: h.s.opts.Retain})
	if err != nil {
		h.s.renderError(w, r, http.StatusBadRequest, "That job cannot be run", err.Error())
		return jobSpec{}, false
	}
	describeJob(r.PostForm, &spec)
	return spec, true
}

func (h jobPages) handleJobReview(w http.ResponseWriter, r *http.Request) {
	spec, ok := h.parseJob(w, r)
	if !ok {
		return
	}
	var what []string
	for _, w := range spec.what {
		if w != "" {
			what = append(what, w)
		}
	}
	rev := jobReview{Title: spec.title, Command: jobCommand(spec.scope), What: what, Items: spec.items,
		Destroy: spec.destroy, Remote: reachesServers(r.PostForm), Fields: carriedJob(r.PostForm),
		Check:    spec.check,
		CanCheck: !spec.check && (r.PostForm.Get("action") == "rotate" || r.PostForm.Get("action") == "rotate-keys")}
	if j := h.s.jobs.Running(); j != nil {
		v, _ := j.View()
		rev.Busy = &v
	}
	rev.Back, rev.BackLabel = jobBack(r.PostForm)
	if !spec.check {
		rev.Options, rev.Kept = jobOptions(r.PostForm, rev.Fields)
	}
	rev.Forced, rev.Backup = checked(r.PostForm, "force") && !spec.check, spec.backup
	rev.Checks = preflights(r.PostForm, spec, rev.Busy != nil)
	rev.Blocked = blocked(rev.Checks)
	h.s.render(w, http.StatusOK, "job_review", h.s.newView(r, "", spec.title, rev))
}

func (h jobPages) handleJobStart(w http.ResponseWriter, r *http.Request) {
	spec, ok := h.parseJob(w, r)
	if !ok || !ownerGate(h).stepUp(w, r) {
		return
	}
	j, err := h.s.jobs.Start(h.s.opts.Engine.Home(), spec.scope, spec.run)
	var busy jobs.ErrBusy
	switch {
	case errors.As(err, &busy):
		http.Redirect(w, r, "/jobs/"+busy.ID, http.StatusSeeOther)
	case err != nil:
		h.s.renderError(w, r, http.StatusConflict, "The job was not started", err.Error())
	default:
		j.SetTitle(spec.title)
		v, _ := j.View()
		http.Redirect(w, r, "/jobs/"+v.ID, http.StatusSeeOther)
	}
}

func (h jobPages) job(w http.ResponseWriter, r *http.Request) *jobs.Job {
	id := r.PathValue("id")
	j := h.s.jobs.Get(id)
	if j == nil || !nameRe.MatchString(id) {
		h.fail(w, r, ErrNotFound)
		return nil
	}
	return j
}

func (h jobPages) handleJob(w http.ResponseWriter, r *http.Request) {
	j := h.job(w, r)
	if j == nil {
		return
	}
	v, _ := j.View()
	v.Summary = pageSummary(v)
	h.s.render(w, http.StatusOK, "job", h.s.newView(r, "", brand.Line(v.Command), v))
}

// pageSummary is the summary a job's page draws: the job's own, or, before
// the job has set one, a summary that is only starting - the keys it has
// planned waiting, or none yet - so the page draws one output from its first
// moment and never the old console in its place.
func pageSummary(v jobs.View) rotator.Summary {
	if sum, ok := v.Summary.(rotator.Summary); ok {
		return sum
	}
	sum := rotator.Summary{Command: strings.TrimSpace(brand.Line(v.Command) + " " + strings.Join(v.Args, " "))}
	if f := strings.Fields(v.Command); len(f) > 0 {
		sum.Verb = f[len(f)-1]
	}
	if v.Progress != nil && (v.State == jobs.Running || v.State == jobs.Waiting) {
		var names []string
		for _, it := range v.Progress.Items {
			names = append(names, it.Name)
		}
		sum.Keys = rotator.Pending(names, "")
	}
	return sum
}

// handleJobEvents streams a job's output as server-sent events: each new
// line as a "line" event, and the job's state as a "state" event whenever it
// changes. The stream ends with the job.
func (h jobPages) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	j := h.job(w, r)
	if j == nil {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	sent, state, progress, items, summary := 0, "", "", "", 0
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		v, changed := j.View()
		for ; sent < len(v.Lines); sent++ {
			writeEvent(w, "line", v.Lines[sent])
		}
		if v.Progress != nil {
			// "done total label": the label is the rest of the line.
			if pr := fmt.Sprintf("%d %d %s", v.Progress.Done, v.Progress.Total, v.Progress.Label); pr != progress {
				progress = pr
				writeEvent(w, "progress", pr)
			}
			// Each item's state, "name=state" joined by spaces: the page
			// marks a key done, running or failed as the job moves on.
			if len(v.Progress.Items) > 0 {
				parts := make([]string, len(v.Progress.Items))
				for i, it := range v.Progress.Items {
					parts[i] = it.Name + "=" + it.State
				}
				if it := strings.Join(parts, " "); it != items {
					items = it
					writeEvent(w, "items", it)
				}
			}
		}
		// The summary's version: the page fetches the summary again when it
		// changes, as a key finishes.
		if v.SummaryVersion != summary {
			summary = v.SummaryVersion
			writeEvent(w, "summary", strconv.Itoa(summary))
		}
		if v.State != state {
			state = v.State
			writeEvent(w, "state", state)
		}
		flusher.Flush()
		if state == jobs.Done || state == jobs.Failed || state == jobs.Cancelled {
			return
		}
		select {
		case <-changed:
		case <-heartbeat.C:
			_, _ = io.WriteString(w, ": still running\n\n")
		case <-r.Context().Done():
			return
		}
	}
}

// writeEvent writes one event. A line holds no newline, so it is one data field.
func writeEvent(w io.Writer, event, data string) {
	data = strings.NewReplacer("\r", "", "\n", " ").Replace(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func (h jobPages) handleJobAnswer(w http.ResponseWriter, r *http.Request) {
	j := h.job(w, r)
	if j == nil {
		return
	}
	v, _ := j.View()
	if !h.s.jobs.Answer(v.ID, r.PostFormValue("answer") == "yes") {
		h.s.renderError(w, r, http.StatusConflict, "Nothing to answer", "The job is not waiting on a question.")
		return
	}
	http.Redirect(w, r, "/jobs/"+v.ID, http.StatusSeeOther)
}

func (h jobPages) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	j := h.job(w, r)
	if j == nil {
		return
	}
	v, _ := j.View()
	h.s.jobs.Cancel(v.ID)
	http.Redirect(w, r, "/jobs/"+v.ID, http.StatusSeeOther)
}

// revokeJob is `authama ssh key revoke` after its confirmation.
func revokeJob(j *jobs.Job, p paths.Paths, ref manifest.KeyRef, reason string) error {
	mutation.LockForMutation(p)
	if err := j.Step(); err != nil {
		return err
	}
	res, err := editor.New(p).Unattended(j).RevokeKey(ref, reason)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(j, "revoked %s (%s)\n", res.Key, res.Fingerprint)
	if len(res.Removed) > 0 {
		_, _ = fmt.Fprintf(j, "  removed from: %s\n", strings.Join(res.Removed, ", "))
	}
	if len(res.NotRemoved) > 0 {
		_, _ = fmt.Fprintf(j, "  NOT removed from (remove it there by hand): %s\n", strings.Join(res.NotRemoved, ", "))
		return errors.New("the key is revoked, but not every target could remove it; see above")
	}
	return nil
}

// backupStep is a rotation's first step: the encrypted backup, or, when it
// is skipped, what that means.
func backupStep(skip bool) string {
	if skip {
		return "writes no encrypted backup: a clean rotation, so a key replaced now can come back only from old/, until the next rotation replaces that too"
	}
	return "writes an encrypted backup of ~/.ssh and the config first, so it needs a backup recipient"
}

// rotateCheckSpec is a dry run of a rotation, `authama ssh rotate ... --check`:
// for each key, every server it is used on is asked whether it answers and the
// current key still logs in, and a target that takes keys by hand is named.
// Nothing changes, so it takes no backup and no snapshot.
func rotateCheckSpec(names, args []string, subjects []history.Subject, h home) jobSpec {
	return jobSpec{
		scope: mutation.Scope{Command: "rotate", Args: args, Source: history.SourceWeb, Subjects: subjects},
		what: []string{
			fmt.Sprintf("tests %d key(s), one at a time, and changes nothing", len(names)),
			"for each server a key is used on: it answers, and the current key still logs in",
			"names each target that takes a new key by hand, where the real rotation will stop and ask",
			"a key that would not rotate cleanly is marked failed, with why",
		},
		run:   func(j *jobs.Job) error { return rotateCheckJob(j, h.p, names) },
		items: names,
		check: true,
	}
}

func rotateCheckJob(j *jobs.Job, p paths.Paths, names []string) error {
	m, inv, err := loadState(p)
	if err != nil {
		return err
	}
	r := rotator.New(p, m, inv)
	j.Plan(names)
	var failed []string
	var reports []rotator.CheckReport
	began := time.Now()
	line := jobLine(j)
	show := func(rest []string, now string) rotator.Summary {
		sum := rotator.SummarizeCheck(reports, time.Since(began))
		sum.Command = line
		sum.Keys = append(sum.Keys, rotator.Pending(rest, now)...)
		j.SetSummary(sum)
		return sum
	}
	defer func() { _ = rotator.SaveFailed(p, show(nil, "").Failed()) }()
	for i, n := range names {
		if err := j.Step(); err != nil {
			return err
		}
		j.Progress(i, len(names), "testing "+n)
		show(names[i:], "testing")
		rep, err := r.Check(n)
		if err != nil {
			reports = append(reports, rotator.CheckReport{Key: n, Problem: err.Error(), ProblemKind: rotator.KindFailed})
			failed = append(failed, n)
			j.Failed(n)
			_, _ = fmt.Fprintf(j, "FAIL %s\n       %v\n", n, err)
			continue
		}
		_, _ = fmt.Fprint(j, rep.Format())
		reports = append(reports, rep)
		if !rep.OK() {
			failed = append(failed, n)
			j.Failed(n)
		}
	}
	j.Progress(len(names), len(names), "")
	if len(failed) > 0 {
		return fmt.Errorf("%d of %d key(s) would not rotate cleanly: %s; nothing was changed", len(failed), len(names), strings.Join(failed, ", "))
	}
	_, _ = fmt.Fprintf(j, "every key would rotate; nothing was changed\n")
	return nil
}

// splitKey is a key selector's profile and key name, as the manifest
// resolves it; the selector as given when it does not resolve.
func splitKey(m *manifest.Manifest, key string) (profile, name string) {
	if ref, err := m.ResolveKeySelector(key); err == nil {
		return ref.Profile, ref.KeyName
	}
	if p, n, ok := strings.Cut(key, "/"); ok {
		return p, n
	}
	return "key", key
}

// jobLine is a job's command line as a terminal would read it:
// "authama ssh rotate --all --check".
func jobLine(j *jobs.Job) string {
	v, _ := j.View()
	return strings.TrimSpace(brand.Line(v.Command) + " " + strings.Join(v.Args, " "))
}

// without is the names in all that are not in some.
func without(all, some []string) []string {
	skip := map[string]bool{}
	for _, n := range some {
		skip[n] = true
	}
	var out []string
	for _, n := range all {
		if !skip[n] {
			out = append(out, n)
		}
	}
	return out
}

// forceStep is what a forced rotation adds, or "" for one that is not.
func forceStep(force bool) string {
	if !force {
		return ""
	}
	return "forced: a key that cannot be rotated in place (never deployed, or its host unreachable) is replaced on this computer instead, its old key moved to old/. Until the new key is deployed, a host that trusted the old one cannot be logged in to with it; deploy it next, or roll back from old/"
}

// replaceFailed is a forced rotation's fallback: each key the rotation could
// not rotate is replaced on this computer instead. It returns the keys still
// failed.
func replaceFailed(j *jobs.Job, p paths.Paths, failed []string, force bool) []string {
	if !force || len(failed) == 0 {
		return failed
	}
	_, _ = fmt.Fprintf(j, "forced: replacing %d key(s) the rotation could not rotate, on this computer\n", len(failed))
	_, still := reconciler.ReplaceLocally(p, failed, platform.EmitUseKeychain(), j)
	return still
}
