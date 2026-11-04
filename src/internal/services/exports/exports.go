// Package exports lays out the lists authama shows as tables for CSV: one set
// of columns per kind of thing, shared by the CLI's --csv and the web UI's
// downloads, so the two cannot drift. No table here ever holds a secret.
package exports

import (
	"strconv"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/query"
	"github.com/simtabi/authama/src/internal/util/csvout"
)

// Hosts is `ssh list`: every host, with its profile.
func Hosts(groups []query.ProfileGroup) csvout.Table {
	t := csvout.Table{Header: []string{"profile", "alias", "hostname", "provider", "key", "status", "tags"}}
	for _, g := range groups {
		for _, r := range g.Rows {
			t.Rows = append(t.Rows, []string{g.Name, r.Alias, r.Hostname, r.ProviderLabel, r.KeyName, r.Status, strings.Join(r.Tags, " ")})
		}
	}
	return t
}

// Keys is `ssh key list`.
func Keys(rows []keysvc.Row) csvout.Table {
	t := csvout.Table{Header: []string{"key", "profile", "type", "fingerprint", "created", "expires", "hosts", "on_disk", "state"}}
	for _, r := range rows {
		t.Rows = append(t.Rows, []string{r.Ref.String(), r.Ref.Profile, r.Type, r.Fingerprint, r.Created, r.ExpiresOn,
			strings.Join(r.Hosts, " "), strconv.FormatBool(r.PrivateOnDisk), r.Status})
	}
	return t
}

// Tokens is `token list`: what is installed, never the secret.
func Tokens(items []tokens.Installed) csvout.Table {
	t := csvout.Table{Header: []string{"tool", "host", "kind", "description", "user", "secret_set", "file"}}
	for _, it := range items {
		t.Rows = append(t.Rows, []string{it.Target, it.Host, it.Kind, it.Description, it.Username, strconv.FormatBool(it.SecretPresent), it.File})
	}
	return t
}

// History is `history`: one row per operation.
func History(ops []history.Op) csvout.Table {
	t := csvout.Table{Header: []string{"op_id", "started_at", "finished_at", "command", "args", "result", "source", "user", "machine", "snapshot"}}
	for _, o := range ops {
		t.Rows = append(t.Rows, []string{o.OpID, o.StartedAt, o.FinishedAt, o.Command, strings.Join(o.Args, " "), o.Result, o.Source, o.OSUser, o.Machine, o.Snapshot})
	}
	return t
}

// Events is `events`: one row per file event.
func Events(evs []events.Event) csvout.Table {
	t := csvout.Table{Header: []string{"time", "seen", "kind", "area", "path", "size", "mode", "detail"}}
	for _, e := range evs {
		t.Rows = append(t.Rows, []string{stamp(e.Time), stamp(e.Seen), e.Kind, e.Area, e.Path,
			strconv.FormatInt(e.Size, 10), e.Mode, e.Detail})
	}
	return t
}

// stamp is a time as RFC 3339 in UTC, or empty when it was never set.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
