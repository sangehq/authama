package web

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/bundler"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/jobs"
	"github.com/sangehq/authama/src/internal/services/lifecycle"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// deleteWithOptionsSpec is `ssh host delete <profile> <alias>` (delete-host)
// or `ssh profile delete <name>` (delete-profile) with --revoke and
// --purge: a job, since revoking reaches every server the keys are deployed
// to. The plain delete stays a previewed change.
func deleteWithOptionsSpec(f url.Values, h home) (jobSpec, error) {
	opt := lifecycle.Options{Revoke: checked(f, "revoke"), Purge: checked(f, "purge")}
	m, err := manifest.Load(h.p.Manifest())
	if err != nil {
		return jobSpec{}, err
	}
	var command, what string
	var args []string
	var subjects []history.Subject
	var run func(lc *lifecycle.Service) (lifecycle.Result, error)
	switch f.Get("action") {
	case "delete-host":
		profile, err := name(f, "profile")
		if err != nil {
			return jobSpec{}, err
		}
		alias, err := name(f, "alias")
		if err != nil {
			return jobSpec{}, err
		}
		if !hasHost(m, profile, alias) {
			return jobSpec{}, badForm("no host %s in profile %s", alias, profile)
		}
		command, args, what = "host delete", []string{profile, alias}, "host "+alias+" ("+profile+")"
		subjects = []history.Subject{{Kind: history.KindProfile, Name: profile}, {Kind: history.KindHost, Name: alias}}
		run = func(lc *lifecycle.Service) (lifecycle.Result, error) { return lc.DeleteHost(profile, alias, opt) }
	default: // delete-profile
		profile, err := name(f, "name")
		if err != nil {
			return jobSpec{}, err
		}
		if _, ok := m.Profiles[profile]; !ok {
			return jobSpec{}, badForm("no profile %s", profile)
		}
		command, args, what = "profile delete", []string{profile}, "profile "+profile+", with every host in it"
		subjects = []history.Subject{{Kind: history.KindProfile, Name: profile}}
		run = func(lc *lifecycle.Service) (lifecycle.Result, error) { return lc.DeleteProfile(profile, opt) }
	}
	steps := []string{"deletes " + what + ": its manifest entry, its block in ~/.ssh/config and its pinned host keys"}
	if opt.Revoke {
		args = append(args, "--revoke")
		steps = append(steps, "first asks every server its keys are deployed to to remove them; one that cannot is named, to remove it there yourself")
	}
	if opt.Purge {
		args = append(args, "--purge")
		steps = append(steps, "deletes the key files too, after writing an encrypted backup")
	} else {
		steps = append(steps, "keeps the key files on disk")
	}
	return jobSpec{
		scope:   mutation.Scope{Command: command, Args: append(args, "--yes"), Source: history.SourceWeb, Subjects: subjects},
		what:    steps,
		title:   "Delete " + what,
		backup:  opt.Purge,
		destroy: true,
		run:     func(j *jobs.Job) error { return deleteWithOptionsJob(j, h.p, h.retain, opt, run) },
	}, nil
}

func deleteWithOptionsJob(j *jobs.Job, p paths.Paths, retain int, opt lifecycle.Options,
	run func(lc *lifecycle.Service) (lifecycle.Result, error)) error {
	mutation.LockForMutation(p)
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
	res, err := run(lifecycle.New(p, platform.EmitUseKeychain()))
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(j, res.Format())
	if len(res.NotRevoked) > 0 {
		return fmt.Errorf("deleted, but not removed from %s: remove the key there by hand", strings.Join(res.NotRevoked, ", "))
	}
	return nil
}

func hasHost(m *manifest.Manifest, profile, alias string) bool {
	for _, h := range m.Profiles[profile].Hosts {
		if h.Alias == alias {
			return true
		}
	}
	return false
}
