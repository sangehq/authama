package dataset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/services/snapshots"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Op is one recorded operation: what ran, who ran it, and what it touched.
type Op struct {
	OpID       string   `json:"op_id"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Result     string   `json:"result"`
	Source     string   `json:"source"` // cli, web, tui, ...
	User       string   `json:"user"`
	Machine    string   `json:"machine"`
	Snapshot   string   `json:"snapshot"`
	Subjects   []string `json:"subjects"` // kind:name each
	Events     []string `json:"events"`   // the audit events it raised
}

// Event is one change the events log saw in a watched file.
type Event struct {
	Time   string `json:"time"` // RFC 3339, UTC
	Seen   string `json:"seen"`
	Kind   string `json:"kind"` // added, changed, removed, mode, started
	Area   string `json:"area"` // ssh, config, auth
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Mode   string `json:"mode"`
	Detail string `json:"detail"`
}

// Snapshot is one snapshot file.
type Snapshot struct {
	Name             string `json:"name"`
	Taken            string `json:"taken"` // RFC 3339, UTC
	Bytes            int64  `json:"bytes"`
	HoldsPrivateKeys bool   `json:"holds_private_keys"` // from before key exclusion
}

// Deployment is one place a recorded key is deployed, or a key deployed
// nowhere (Target empty), as `ssh audit --csv` lists them.
type Deployment struct {
	KeyPath     string `json:"key_path"`
	Fingerprint string `json:"fingerprint"`
	Status      string `json:"status"` // deployed or needs-redeploy
	Target      string `json:"target"`
	Method      string `json:"method"`
	Verified    bool   `json:"verified"`
	Date        string `json:"date"`
}

// readRecords reads the optional parts asked for.
func readRecords(ds *Dataset, p paths.Paths, o Options, inv *inventory.Inventory) error {
	if o.wants(PartOps) {
		ops, err := readOps(p, o.History)
		if err != nil {
			return fmt.Errorf("history: %w", err)
		}
		ds.Ops = ops
	}
	if o.wants(PartEvents) {
		evs, err := events.List(p, o.Events)
		if err != nil {
			return fmt.Errorf("events: %w", err)
		}
		for _, e := range evs {
			ds.Events = append(ds.Events, Event{Time: stamp(e.Time), Seen: stamp(e.Seen), Kind: e.Kind, Area: e.Area,
				Path: e.Path, Size: e.Size, Mode: e.Mode, Detail: e.Detail})
		}
	}
	if o.wants(PartSnapshots) {
		for _, s := range snapshots.List(p.SnapshotsDir()) {
			var taken time.Time
			var size int64
			if fi, err := os.Stat(s); err == nil {
				taken, size = fi.ModTime(), fi.Size()
			}
			ds.Snapshots = append(ds.Snapshots, Snapshot{Name: filepath.Base(s), Taken: stamp(taken), Bytes: size,
				HoldsPrivateKeys: snapshots.HoldsKeyMaterial(s)})
		}
	}
	if o.wants(PartDeployments) {
		ds.Deployments = deploymentsOf(inv)
	}
	return nil
}

// readOps reads the history store; a home that has none has no operations.
func readOps(p paths.Paths, f history.Filter) ([]Op, error) {
	if _, err := os.Stat(p.HistoryDB()); err != nil {
		return []Op{}, nil
	}
	s, err := history.Open(p.HistoryDB())
	if err != nil && !errors.Is(err, history.ErrNewerSchema) {
		return nil, err
	}
	if s == nil {
		return []Op{}, nil
	}
	defer func() { _ = s.Close() }()
	ops, err := s.Query(f)
	if err != nil {
		return nil, err
	}
	out := make([]Op, 0, len(ops))
	for _, op := range ops {
		subs := make([]string, 0, len(op.Subjects))
		for _, sub := range op.Subjects {
			subs = append(subs, sub.Kind+":"+sub.Name)
		}
		evs := make([]string, 0, len(op.Events))
		for _, e := range op.Events {
			evs = append(evs, e.Event)
		}
		args := op.Args
		if args == nil {
			args = []string{}
		}
		out = append(out, Op{OpID: op.OpID, StartedAt: op.StartedAt, FinishedAt: op.FinishedAt, Command: op.Command,
			Args: args, Result: op.Result, Source: op.Source, User: op.OSUser, Machine: op.Machine,
			Snapshot: op.Snapshot, Subjects: subs, Events: evs})
	}
	return out, nil
}

// deploymentsOf lists the inventory's deployments, ordered by key path, as
// `ssh audit --csv` does.
func deploymentsOf(inv *inventory.Inventory) []Deployment {
	out := []Deployment{}
	if inv == nil {
		return out
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
			out = append(out, Deployment{KeyPath: rec.Path, Fingerprint: fp, Status: status})
		}
		for _, d := range rec.Deployments {
			date := ""
			if d.Date != nil {
				date = *d.Date
			}
			out = append(out, Deployment{KeyPath: rec.Path, Fingerprint: fp, Status: status, Target: d.Target,
				Method: d.Method, Verified: d.Verified, Date: date})
		}
	}
	return out
}

// stamp is a time as RFC 3339 in UTC, or "" when it was never set.
func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
