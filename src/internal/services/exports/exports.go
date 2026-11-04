// Package exports lays out the lists authama shows as tables for CSV: one set
// of columns per kind of thing, shared by the CLI's --csv and the web UI's
// downloads, so the two cannot drift. No table here ever holds a secret.
package exports

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/tokens"
	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/query"
	"github.com/simtabi/authama/src/internal/services/snapshots"
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

// History is `history`: one row per operation, with what it was about
// ("kind:name" each) and the audit events it raised, so a reviewer can see
// "token.revealed" against npm/registry.npmjs.org without opening the op.
func History(ops []history.Op) csvout.Table {
	t := csvout.Table{Header: []string{"op_id", "started_at", "finished_at", "command", "args", "result", "source", "user", "machine", "snapshot", "subjects", "events"}}
	for _, o := range ops {
		subs := make([]string, 0, len(o.Subjects))
		for _, s := range o.Subjects {
			subs = append(subs, s.Kind+":"+s.Name)
		}
		evs := make([]string, 0, len(o.Events))
		for _, e := range o.Events {
			evs = append(evs, e.Event)
		}
		t.Rows = append(t.Rows, []string{o.OpID, o.StartedAt, o.FinishedAt, o.Command, strings.Join(o.Args, " "), o.Result, o.Source,
			o.OSUser, o.Machine, o.Snapshot, strings.Join(subs, " "), strings.Join(evs, " ")})
	}
	return t
}

// Deployments is `ssh audit --csv`: one row per place each key is deployed,
// and one row with no target for a key deployed nowhere, ordered by key path.
func Deployments(inv *inventory.Inventory) csvout.Table {
	t := csvout.Table{Header: []string{"key_path", "fingerprint", "status", "target", "method", "verified", "date"}}
	if inv == nil {
		return t
	}
	fps := make([]string, 0, len(inv.Keys))
	for fp := range inv.Keys {
		fps = append(fps, fp)
	}
	sort.Slice(fps, func(i, j int) bool {
		a, b := inv.Keys[fps[i]], inv.Keys[fps[j]]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return fps[i] < fps[j]
	})
	for _, fp := range fps {
		rec := inv.Keys[fp]
		status := "deployed"
		if rec.NeedsRedeploy() {
			status = "needs-redeploy"
		}
		if len(rec.Deployments) == 0 {
			t.Rows = append(t.Rows, []string{rec.Path, fp, status, "", "", "", ""})
		}
		for _, d := range rec.Deployments {
			date := ""
			if d.Date != nil {
				date = *d.Date
			}
			t.Rows = append(t.Rows, []string{rec.Path, fp, status, d.Target, d.Method, strconv.FormatBool(d.Verified), date})
		}
	}
	return t
}

// Snapshots is `ssh snapshots list --csv`: every snapshot file in list,
// oldest first as list orders them, with its size and whether it predates
// key exclusion and so holds private keys.
func Snapshots(list []string) csvout.Table {
	t := csvout.Table{Header: []string{"snapshot", "taken", "bytes", "holds_private_keys"}}
	for _, s := range list {
		var taken time.Time
		var size int64
		if fi, err := os.Stat(s); err == nil {
			taken, size = fi.ModTime(), fi.Size()
		}
		t.Rows = append(t.Rows, []string{filepath.Base(s), stamp(taken), strconv.FormatInt(size, 10),
			strconv.FormatBool(snapshots.HoldsKeyMaterial(s))})
	}
	return t
}

// Profile is one row of Profiles: what the profiles page shows of each.
type Profile struct {
	Name, Category, KeyScope, KeyStore, KeyComment string
	Hosts, Keys                                    int
}

// Profiles is the profiles page's list, one row per profile.
func Profiles(rows []Profile) csvout.Table {
	t := csvout.Table{Header: []string{"profile", "category", "key_scope", "key_store", "hosts", "keys", "key_comment"}}
	for _, r := range rows {
		t.Rows = append(t.Rows, []string{r.Name, r.Category, r.KeyScope, r.KeyStore, strconv.Itoa(r.Hosts), strconv.Itoa(r.Keys), r.KeyComment})
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
