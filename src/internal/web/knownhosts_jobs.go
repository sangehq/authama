package web

import (
	"fmt"
	"github.com/simtabi/authama/src/internal/services/rotator"
	"net/url"
	"strings"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/services/knownhosts"
	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// knownhostsInitSpec is `authama ssh knownhosts init <profile>|--all [--force] --yes`.
func knownhostsInitSpec(f url.Values, h home, retain int) (jobSpec, error) {
	m, err := manifest.Load(h.p.Manifest())
	if err != nil {
		return jobSpec{}, err
	}
	all, force := checked(f, "all"), checked(f, "force")
	profile, args := "", []string{"--all"}
	var subjects []history.Subject
	if !all {
		if profile, err = name(f, "profile"); err != nil {
			return jobSpec{}, err
		}
		if _, ok := m.Profiles[profile]; !ok {
			return jobSpec{}, badForm("no profile %q in the manifest", profile)
		}
		args = []string{profile}
		subjects = []history.Subject{{Kind: history.KindProfile, Name: profile}}
	}
	if force {
		args = append(args, "--force")
	}
	scope := "every profile's hosts"
	if !all {
		scope = profile + "'s hosts"
	}
	return jobSpec{
		scope: mutation.Scope{Command: "knownhosts init", Args: append(args, "--yes"), Source: history.SourceWeb, Subjects: subjects},
		what: []string{
			knownhosts.InitNotice,
			"scans " + scope + "; a host that does not answer is reported and left for later",
			"skips a host already in known_hosts unless re-scanning is ticked, which adds any new keys",
			"snapshots ~/.ssh first, and lists every pinned fingerprint to check",
		},
		run: func(j *jobs.Job) error { return knownhostsInitJob(j, h.p, retain, profile, all, force) },
	}, nil
}

func knownhostsInitJob(j *jobs.Job, p paths.Paths, retain int, profile string, all, force bool) error {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return err
	}
	mutation.SnapshotBeforeMutation(p, retain)
	if err := j.Step(); err != nil {
		return err
	}
	scope := profile
	if all {
		scope = "--all"
	}
	sum := newSummaryOf(j, "pin", "authama ssh knownhosts init "+scope,
		"Hosts marked not pinned are not trusted yet: ssh asks before it connects to them.")
	report, err := knownhosts.New(p.SSHDir).InitEach(m, profile, all, force, func(done, total int, alias string) {
		label := ""
		if alias != "" {
			label = "scanning " + alias
			sum.pending("hosts/"+alias, "scanning")
		}
		j.Progress(done, total, label)
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(j, report.Format())
	sum.show(pinRows(report.Results))
	return nil
}

// knownhostsPinSpec is `authama ssh knownhosts pin <alias>`, for a host in the
// manifest only: the web never sends a free hostname or port to ssh-keyscan.
func knownhostsPinSpec(f url.Values, h home, retain int) (jobSpec, error) {
	alias, err := name(f, "host")
	if err != nil {
		return jobSpec{}, err
	}
	m, err := manifest.Load(h.p.Manifest())
	if err != nil {
		return jobSpec{}, err
	}
	targets, err := knownhosts.Targets(m)
	if err != nil {
		return jobSpec{}, err
	}
	var target *knownhosts.Target
	for i := range targets {
		if targets[i].Alias == alias {
			target = &targets[i]
			break
		}
	}
	if target == nil {
		return jobSpec{}, badForm("no host %q in the manifest", alias)
	}
	t := *target
	return jobSpec{
		scope: mutation.Scope{Command: "knownhosts pin", Args: []string{alias, "--yes"}, Source: history.SourceWeb,
			Subjects: []history.Subject{{Kind: history.KindProfile, Name: t.Profile}, {Kind: history.KindHost, Name: alias}}},
		what: []string{
			fmt.Sprintf("asks %s (%s, port %d) for its host keys with ssh-keyscan", alias, t.Hostname, t.Port),
			"shows each key's fingerprint and waits: nothing is written until you trust them",
			"then adds them to ~/.ssh/known_hosts, snapshotting ~/.ssh first",
		},
		run: func(j *jobs.Job) error { return knownhostsPinJob(j, h.p, retain, t) },
	}, nil
}

func knownhostsPinJob(j *jobs.Job, p paths.Paths, retain int, t knownhosts.Target) error {
	svc := knownhosts.New(p.SSHDir)
	keys, err := svc.ScanChecked(t.Hostname, t.Port)
	if err != nil {
		return err
	}
	label := t.Profile
	if label == "" {
		label = "global"
	}
	fps := make([]string, 0, len(keys))
	for _, sk := range keys {
		line := fmt.Sprintf("[%s] %s  %s  %s", label, sk.Host, sk.Keytype, sk.Fingerprint)
		_, _ = fmt.Fprintln(j, line)
		fps = append(fps, sk.Keytype+"  "+sk.Fingerprint)
	}
	owner := t.Profile
	if owner == "" {
		owner = "global"
	}
	sum := newSummaryOf(j, "pin", "", "")
	row := rotator.SummaryKey{Ref: owner + "/" + t.Alias, Key: fmt.Sprintf("%s:%d", t.Hostname, t.Port), Via: "ssh-keyscan",
		Result: "waiting", Pending: true}
	sum.show([]rotator.SummaryKey{row})
	if !j.AskWith(jobs.Question{
		Prompt: fmt.Sprintf("Trust these %d host key(s) for %s? Check them against a fingerprint you got another way.", len(keys), t.Alias),
		Text:   strings.Join(fps, "\n"),
		Yes:    "Trust these keys",
		No:     "Don't trust them",
	}) {
		_, _ = fmt.Fprintln(j, "pinned 0 host key(s) into known_hosts")
		row.Pending, row.Result, row.Severity, row.Kind = false, "not trusted", 1, rotator.KindFailed
		row.Note = "you did not trust the keys shown, so nothing was pinned: ssh asks before it connects"
		sum.show([]rotator.SummaryKey{row})
		return nil
	}
	if err := j.Step(); err != nil {
		return err
	}
	mutation.SnapshotBeforeMutation(p, retain)
	lines := make([]string, 0, len(keys))
	for _, sk := range keys {
		lines = append(lines, sk.Line)
	}
	total, err := svc.Add(lines)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(j, "pinned %d host key(s) into known_hosts\n", total)
	row.Pending, row.Result = false, "pinned"
	row.Note = fmt.Sprintf("pinned %d host key(s) into known_hosts", len(keys))
	sum.show([]rotator.SummaryKey{row})
	return nil
}
