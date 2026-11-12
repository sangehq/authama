// Package feed is authama's event feed: one ordered stream of what happened,
// for the API, webhooks and MCP. An operation that finished (any change, made
// from the CLI, the TUI or the web) is an `operation.finished` event; a change
// to a watched file is a `file.<kind>` event.
//
// It is read from the history database and the file events log, so it holds
// changes made by every authama process, not only the one serving the feed.
// Events are ordered by time, then id; a cursor names a place in that order.
package feed

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// The event types. A file event's type is "file." and its kind (added,
// changed, removed, mode, started).
const (
	TypeOperation  = "operation.finished"
	TypeFilePrefix = "file."
)

// Event is one thing that happened.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// Cursor is the event's place in the feed: the next List after it starts
// after this event.
func (e Event) Cursor() string { return e.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + e.ID }

// Operation is an operation.finished event's data.
type Operation struct {
	OpID     string   `json:"op_id"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Source   string   `json:"source"`
	Result   string   `json:"result"`
	Error    string   `json:"error,omitempty"`
	Subjects []string `json:"subjects,omitempty"`
	Snapshot string   `json:"snapshot,omitempty"`
	OSUser   string   `json:"os_user,omitempty"`
	Machine  string   `json:"machine,omitempty"`
}

// File is a file.<kind> event's data. The event's timestamp is when authama
// noticed the change; ChangedAt is when the file says it changed, which can
// be much earlier (a change made while nothing was watching).
type File struct {
	Kind      string    `json:"kind"`
	ChangedAt time.Time `json:"changed_at"`
	Area      string    `json:"area"`
	Path      string    `json:"path"`
	Size      int64     `json:"size,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// MaxLimit is the most events one List returns.
const MaxLimit = 500

// lookback is how far before the cursor operations are read: the history is
// filtered by when an operation started, and the feed orders by when it
// finished, so a long one (a rotation that waited for a paste) is not missed.
const lookback = 48 * time.Hour

// ErrBadCursor is a cursor this feed did not give.
var ErrBadCursor = errors.New("not a cursor this feed gave: pass the cursor of the last event you read, or none")

// Now is a cursor at now: List after it returns only what happens from now
// on. A new webhook starts here, so it is not sent the whole history.
func Now(now time.Time) string { return now.UTC().Format(time.RFC3339Nano) + "|" }

// List returns up to limit events after cursor, oldest first, and the cursor
// to pass next (the last event's, or cursor itself when there are none). An
// empty cursor lists the newest limit events.
func List(p paths.Paths, cursor string, limit int) ([]Event, string, error) {
	if limit <= 0 || limit > MaxLimit {
		limit = MaxLimit
	}
	var at time.Time
	var atID string
	if cursor != "" {
		ts, id, ok := strings.Cut(cursor, "|")
		t, err := time.Parse(time.RFC3339Nano, ts)
		if !ok || err != nil {
			return nil, cursor, ErrBadCursor
		}
		at, atID = t, id
	}
	all, err := read(p, at)
	if err != nil {
		return nil, cursor, err
	}
	slices.SortStableFunc(all, func(a, b Event) int {
		if c := a.Timestamp.Compare(b.Timestamp); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	var out []Event
	for _, e := range all {
		if cursor != "" && !after(e, at, atID) {
			continue
		}
		out = append(out, e)
	}
	if cursor == "" && len(out) > limit {
		out = out[len(out)-limit:]
	}
	if len(out) > limit {
		out = out[:limit]
	}
	next := cursor
	if len(out) > 0 {
		next = out[len(out)-1].Cursor()
	}
	return out, next, nil
}

func after(e Event, at time.Time, id string) bool {
	if c := e.Timestamp.Compare(at); c != 0 {
		return c > 0
	}
	return e.ID > id
}

// read gathers the operations and file events from at on (all of them when
// at is zero).
func read(p paths.Paths, at time.Time) ([]Event, error) {
	var out []Event
	ops, err := operations(p, at)
	if err != nil {
		return nil, err
	}
	out = append(out, ops...)
	f := events.Filter{}
	if !at.IsZero() {
		f.Since = at
	}
	evs, err := events.List(p, f)
	if err != nil {
		return nil, fmt.Errorf("file events: %w", err)
	}
	for _, ev := range evs {
		out = append(out, fileEvent(ev))
	}
	return out, nil
}

func operations(p paths.Paths, at time.Time) ([]Event, error) {
	if _, err := os.Stat(p.HistoryDB()); err != nil {
		return nil, nil
	}
	s, err := history.Open(p.HistoryDB())
	if err != nil && !errors.Is(err, history.ErrNewerSchema) {
		return nil, fmt.Errorf("history: %w", err)
	}
	if s == nil {
		return nil, nil
	}
	defer func() { _ = s.Close() }()
	f := history.Filter{}
	if !at.IsZero() {
		f.Since = at.Add(-lookback).UTC().Format(history.TimeFormat)
	}
	ops, err := s.Query(f)
	if err != nil {
		return nil, fmt.Errorf("history: %w", err)
	}
	var out []Event
	for _, op := range ops {
		if op.FinishedAt == "" {
			continue // still running: it is an event once it finishes
		}
		t, err := time.Parse(history.TimeFormat, op.FinishedAt)
		if err != nil {
			continue
		}
		subs := make([]string, 0, len(op.Subjects))
		for _, sub := range op.Subjects {
			subs = append(subs, sub.Kind+":"+sub.Name)
		}
		out = append(out, Event{ID: "op_" + op.OpID, Type: TypeOperation, Timestamp: t.UTC(), Data: Operation{
			OpID: op.OpID, Command: op.Command, Args: nonNil(op.Args), Source: op.Source, Result: op.Result, Error: op.Error,
			Subjects: subs, Snapshot: op.Snapshot, OSUser: op.OSUser, Machine: op.Machine,
		}})
	}
	return out, nil
}

func fileEvent(ev events.Event) Event {
	id := ev.Hash
	if id == "" {
		sum := sha256.Sum256([]byte(ev.Time.String() + ev.Kind + ev.Area + ev.Path + ev.Detail))
		id = hex.EncodeToString(sum[:])
	}
	if len(id) > 24 {
		id = id[:24]
	}
	// Ordered by when it was seen, not when the file changed: a change noticed
	// now comes after a reader's cursor even if the file is old, so it is
	// never skipped.
	seen := ev.Seen
	if seen.IsZero() {
		seen = ev.Time
	}
	return Event{ID: "file_" + id, Type: TypeFilePrefix + ev.Kind, Timestamp: seen.UTC(), Data: File{
		Kind: ev.Kind, ChangedAt: ev.Time.UTC(), Area: ev.Area, Path: ev.Path, Size: ev.Size, Mode: ev.Mode, Detail: ev.Detail,
	}}
}

// Matches reports whether e's type starts with one of prefixes; no prefixes
// match every event.
func Matches(e Event, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	return slices.ContainsFunc(prefixes, func(pre string) bool { return strings.HasPrefix(e.Type, pre) })
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
