// Package feed is authama's event feed: one ordered stream of what happened,
// for the API, webhooks and MCP, each event a CloudEvents 1.0 event in
// structured JSON (https://github.com/cloudevents/spec). An operation that
// finished (any change, made from the CLI or the web) is a
// com.simtabi.authama.operation.finished event; a change to a watched file is
// com.simtabi.authama.file.added, .changed, .removed or .mode-changed.
//
// The envelope is sange's (its design, webhooks-and-events.md section 3.1),
// so one receiver reads both: the type is reverse-DNS from authama's own
// prefix, as its desktop notifier's app id is; the source is a URL naming
// this install by a random id, never the host or the user; a path in a home
// is written from ~, so neither a subject nor the data names the account.
//
// It is read from the history database and the file events log, so it holds
// changes made by every authama process, not only the one serving the feed.
// Events are ordered by time, then id; a cursor names a place in that order.
package feed

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// TypePrefix starts every event type: reverse-DNS, as authama's desktop
// notifier's app id is (com.simtabi.authama).
const TypePrefix = "com.simtabi.authama."

// The event types.
const (
	TypeOperation     = TypePrefix + "operation.finished"
	TypeFileAdded     = TypePrefix + "file.added"
	TypeFileChanged   = TypePrefix + "file.changed"
	TypeFileRemoved   = TypePrefix + "file.removed"
	TypeFileMode      = TypePrefix + "file.mode-changed"
	TypeWatchStarted  = TypePrefix + "file.watch.started"
	TypeWebhookTested = TypePrefix + "webhook.tested"
	sourceBase        = "https://opensource.simtabi.com/products/simtabi/authama/install/"
	specVersion       = "1.0"
	jsonContentType   = "application/json"
)

// fileTypes maps a file event's kind to its type.
var fileTypes = map[string]string{
	events.KindAdded: TypeFileAdded, events.KindChanged: TypeFileChanged, events.KindRemoved: TypeFileRemoved,
	events.KindMode: TypeFileMode, events.KindStarted: TypeWatchStarted,
}

// Event is one thing that happened: a CloudEvents 1.0 event.
type Event struct {
	SpecVersion     string    `json:"specversion"`
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	Subject         string    `json:"subject,omitempty"`
	Time            time.Time `json:"time"`
	DataContentType string    `json:"datacontenttype"`
	Data            any       `json:"data"`
	// Sub is the authamasub extension: set per webhook delivery to the
	// endpoint's name, inside the signed body, so a delivery captured from
	// one endpoint cannot be replayed to another that checks it.
	Sub string `json:"authamasub,omitempty"`
}

// Cursor is the event's place in the feed: the next List after it starts
// after this event.
func (e Event) Cursor() string { return e.Time.UTC().Format(time.RFC3339Nano) + "|" + e.ID }

// New is an event of typ at t, from this install, with its data.
func New(p paths.Paths, id, typ, subject string, t time.Time, data any) Event {
	return Event{SpecVersion: specVersion, ID: id, Source: Source(p), Type: typ, Subject: subject, Time: t.UTC(),
		DataContentType: jsonContentType, Data: data}
}

// Source is this install's CloudEvents source: a URL naming it by a random
// id, made the first time it is asked for and kept in the state home. It
// names neither the host nor the user.
func Source(p paths.Paths) string { return sourceBase + installID(p) }

func installID(p paths.Paths) string {
	f := filepath.Join(p.StateDir(), "install-id")
	if b, err := os.ReadFile(f); err == nil {
		if id := strings.TrimSpace(string(b)); len(id) == 16 {
			return id
		}
	}
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	id := hex.EncodeToString(raw)
	if fs.EnsureDir(p.StateDir(), 0o700) == nil {
		_ = fs.WriteTextAtomic(f, id+"\n", 0o600)
	}
	return id
}

// Operation is an operation.finished event's data.
type Operation struct {
	OpID    string   `json:"op_id"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// MadeFrom is where the change was made: cli or web (the history's
	// source; named apart from the event's own source).
	MadeFrom string   `json:"made_from"`
	Result   string   `json:"result"`
	Error    string   `json:"error,omitempty"`
	Subjects []string `json:"subjects,omitempty"`
	Snapshot string   `json:"snapshot,omitempty"`
}

// File is a file event's data. The event's time is when authama
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
		if c := a.Time.Compare(b.Time); c != 0 {
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
	if c := e.Time.Compare(at); c != 0 {
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
		out = append(out, fileEvent(p, ev))
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
		out = append(out, New(p, "op_"+op.OpID, TypeOperation, "operation/"+op.OpID, t, Operation{
			OpID: op.OpID, Command: op.Command, Args: nonNil(op.Args), MadeFrom: op.Source, Result: op.Result, Error: op.Error,
			Subjects: subs, Snapshot: op.Snapshot,
		}))
	}
	return out, nil
}

func fileEvent(p paths.Paths, ev events.Event) Event {
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
	typ, ok := fileTypes[ev.Kind]
	if !ok {
		typ = TypePrefix + "file." + ev.Kind
	}
	path := fromHome(ev.Path)
	subject := ""
	if path != "" {
		subject = "file/" + path
	}
	return New(p, "file_"+id, typ, subject, seen, File{
		Kind: ev.Kind, ChangedAt: ev.Time.UTC(), Area: ev.Area, Path: path, Size: ev.Size, Mode: ev.Mode, Detail: ev.Detail,
	})
}

// fromHome writes a path in the user's home from ~, so an event never names
// the account (/Users/<name>) it happened in.
func fromHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}

// Matches reports whether e's type starts with one of prefixes, each given
// whole (com.simtabi.authama.file.) or without the common prefix (file.); no
// prefixes match every event.
func Matches(e Event, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	short := strings.TrimPrefix(e.Type, TypePrefix)
	return slices.ContainsFunc(prefixes, func(pre string) bool {
		return strings.HasPrefix(e.Type, pre) || strings.HasPrefix(short, pre)
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
