package feed

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/events"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// home writes two finished operations, one still running, and two file
// events, at known times.
func home(t *testing.T) paths.Paths {
	t.Helper()
	p := paths.Paths{ConfigDir: t.TempDir()}
	s, err := history.Open(p.HistoryDB())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	for _, op := range []history.Op{
		{OpID: "a1", StartedAt: "2026-11-12T10:00:00Z", FinishedAt: "2026-11-12T10:00:05Z", Command: "host add", Args: []string{"box"}, Source: "cli", Result: "ok",
			Subjects: []history.Subject{{Kind: history.KindProfile, Name: "work"}}},
		{OpID: "a3", StartedAt: "2026-11-12T10:02:00Z", FinishedAt: "2026-11-12T10:02:01Z", Command: "ssh rotate", Source: "web", Result: "error", Error: "box:22 unreachable"},
	} {
		op := op
		if err := s.Begin(&op); err != nil {
			t.Fatal(err)
		}
		if err := s.Finish(&op); err != nil {
			t.Fatal(err)
		}
	}
	running := history.Op{OpID: "a4", StartedAt: "2026-11-12T10:03:00Z", Command: "ssh rotate", Source: "web"}
	if err := s.Begin(&running); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(events.LogFile(p)), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(events.LogFile(p))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, ev := range []events.Event{
		{Time: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC), Seen: time.Date(2026, 11, 12, 10, 1, 0, 0, time.UTC), Kind: "changed", Area: "ssh", Path: "config", Hash: "h1"},
		// Changed long before it was noticed: ordered by when it was seen.
		{Time: time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC), Seen: time.Date(2026, 11, 12, 10, 4, 0, 0, time.UTC), Kind: "added", Area: "ssh", Path: "profiles/work/work_box-ed25519", Hash: "h2"},
	} {
		if err := enc.Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
	return p
}

// The feed interleaves finished operations and file events by time, leaves
// out an operation still running, and a cursor resumes after the last event
// read, so a reader that keeps its cursor sees each event once.
func TestTheFeedOrdersEventsAndResumesFromACursor(t *testing.T) {
	p := home(t)
	all, next, err := List(p, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range all {
		got = append(got, e.ID+" "+e.Type)
	}
	want := []string{"op_a1 " + TypeOperation, "file_h1 " + TypeFileChanged, "op_a3 " + TypeOperation, "file_h2 " + TypeFileAdded}
	if len(got) != len(want) {
		t.Fatalf("feed %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("feed %v, want %v", got, want)
		}
	}
	if op := all[2].Data.(Operation); op.Result != "error" || op.Error == "" || op.MadeFrom != "web" {
		t.Errorf("operation data %+v", op)
	}
	if op := all[0].Data.(Operation); len(op.Subjects) != 1 || op.Subjects[0] != "profile:work" {
		t.Errorf("subjects %v", op.Subjects)
	}

	page, cur, err := List(p, all[1].Cursor(), 1)
	if err != nil || len(page) != 1 || page[0].ID != "op_a3" {
		t.Fatalf("after the second event, limit 1: %v %v", page, err)
	}
	rest, cur, _ := List(p, cur, 0)
	if len(rest) != 1 || rest[0].ID != "file_h2" || cur != next {
		t.Fatalf("resuming gave %v, cursor %q want %q", rest, cur, next)
	}
	if none, same, _ := List(p, next, 0); len(none) != 0 || same != next {
		t.Errorf("past the end gave %v and moved the cursor to %q", none, same)
	}
	// A cursor at now sees nothing that already happened.
	if none, _, _ := List(p, Now(time.Date(2026, 11, 12, 11, 0, 0, 0, time.UTC)), 0); len(none) != 0 {
		t.Errorf("a cursor at now listed %v", none)
	}
	if _, _, err := List(p, "not-a-cursor", 0); !errors.Is(err, ErrBadCursor) {
		t.Errorf("a made-up cursor gave %v", err)
	}
	// Each is a CloudEvents 1.0 event from this install, the same source
	// every time, and names no account.
	for _, e := range all {
		if e.SpecVersion != "1.0" || e.DataContentType != "application/json" || e.Subject == "" ||
			!strings.HasPrefix(e.Source, "https://opensource.simtabi.com/products/simtabi/authama/install/") || e.Source != all[0].Source {
			t.Errorf("not a CloudEvent from this install: %+v", e)
		}
	}
	if all[0].Subject != "operation/a1" {
		t.Errorf("an operation's subject is %q", all[0].Subject)
	}
	if !Matches(all[1], []string{"file."}) || !Matches(all[1], []string{TypePrefix + "file."}) || Matches(all[0], []string{"file."}) || !Matches(all[0], nil) {
		t.Error("type prefixes do not filter as they should")
	}
}

// A home with no history and no file events has an empty feed, not an error.
func TestAnEmptyHomeHasAnEmptyFeed(t *testing.T) {
	evs, next, err := List(paths.Paths{ConfigDir: t.TempDir()}, "", 0)
	if err != nil || len(evs) != 0 || next != "" {
		t.Fatalf("%v %q %v", evs, next, err)
	}
}

// A path in the home is written from ~, so an event leaving the machine never
// names the account; a path elsewhere is left as it is.
func TestAPathInTheHomeIsWrittenFromTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got := fromHome(filepath.Join(home, ".ssh", "config")); got != "~/.ssh/config" {
		t.Errorf("got %q", got)
	}
	if got := fromHome(home); got != "~" {
		t.Errorf("the home itself is %q", got)
	}
	if got := fromHome("/etc/ssh/ssh_config"); got != "/etc/ssh/ssh_config" {
		t.Errorf("a path outside the home became %q", got)
	}
	if got := fromHome(home + "-other/x"); got != home+"-other/x" {
		t.Errorf("a sibling of the home became %q", got)
	}
}

// The source is made once and kept: every event from an install has the same.
func TestTheSourceIsMadeOnceAndKept(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	a, b := Source(p), Source(p)
	if a != b || len(strings.TrimPrefix(a, sourceBase)) != 16 {
		t.Fatalf("sources %q and %q", a, b)
	}
	if other := Source(paths.Paths{ConfigDir: t.TempDir()}); other == a {
		t.Error("two installs share a source")
	}
}
