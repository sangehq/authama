package web

import (
	"context"
	"github.com/sangehq/authama/src/internal/util/applog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/sangehq/authama/src/internal/services/events"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// defaultEventInterval is how often a running server scans for file changes.
const defaultEventInterval = 30 * time.Second

// watchEvents scans once at start, then every interval, until ctx ends. A
// failed scan is tried again next time: the page says what the last one found.
func (s *Server) watchEvents(ctx context.Context) {
	every := s.opts.EventInterval
	if every <= 0 {
		every = defaultEventInterval
	}
	s.scanEvents()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.scanEvents()
		}
	}
}

// scanEvents records what changed since the last scan; nothing without roots.
func (s *Server) scanEvents() {
	if s.opts.EventRoots == nil {
		return
	}
	p := s.opts.Engine.Home()
	evs, err := events.Scan(p, s.opts.EventRoots(p), s.now().UTC())
	if err != nil {
		applog.Warn("file scan failed", "area", "watch", "error", err)
		return
	}
	for _, e := range evs {
		applog.Info("file changed", "area", "watch", "kind", e.Kind, "file", e.Path, "where", e.Area)
	}
	checkStructure(&s.structureSeen, p, evs, s.now())
}

// checkStructure refreshes the structure suggestions (the structure page's
// list) after a scan that saw a file change in ~/.ssh or the config home,
// and once at start. It only suggests: nothing moves until the owner
// approves it.
//
// A function, not a Server method: Server's method count is kept down on
// purpose (TestNoTypeHasBecomeTheFacadeAgain).
func checkStructure(seen *atomic.Bool, p paths.Paths, evs []events.Event, now time.Time) {
	changed := !seen.Load()
	for _, e := range evs {
		if e.Area == events.AreaSSH || e.Area == events.AreaConfig {
			changed = true
			break
		}
	}
	if !changed {
		return
	}
	if _, _, err := structure.Check(p, now); err == nil {
		seen.Store(true)
	}
}

// EventRow is one file event, with the authama operation that made it when
// one was running then.
type EventRow struct {
	events.Event
	OpID, Command string
}

// EventsView is the events page.
type EventsView struct {
	Rows       []EventRow
	Area, Kind string
	Tracking   bool // the server tracks files; false when it was started without roots
	Counts     map[string]int
	Integrity  events.Integrity
}

// recentEvents reads the newest events, each matched to the operation it fell
// inside, if any: an operation's start to two seconds after it finished.
func (s *Server) recentEvents(f events.Filter) []EventRow {
	evs, err := events.List(s.opts.Engine.Home(), f)
	if err != nil || len(evs) == 0 {
		return nil
	}
	ops, _ := s.opts.Engine.History(history.Filter{Limit: 200})
	rows := make([]EventRow, 0, len(evs))
	for _, e := range evs {
		r := EventRow{Event: e}
		if e.Path == "" {
			rows = append(rows, r) // "started" names no file, so no operation made it
			continue
		}
		for _, op := range ops {
			start, err1 := time.Parse(history.TimeFormat, op.StartedAt)
			end, err2 := time.Parse(history.TimeFormat, op.FinishedAt)
			if err1 != nil {
				continue
			}
			if err2 != nil {
				end = start
			}
			if !e.Time.Before(start.Add(-time.Second)) && !e.Time.After(end.Add(2*time.Second)) {
				r.OpID, r.Command = op.OpID, op.Command
				break
			}
		}
		rows = append(rows, r)
	}
	return rows
}

func (h systemPages) handleEvents(w http.ResponseWriter, r *http.Request) {
	h.s.scanEvents()
	q := r.URL.Query()
	f := eventsFilter(q)
	v := EventsView{Area: f.Area, Kind: f.Kind, Tracking: h.s.opts.EventRoots != nil, Counts: map[string]int{}}
	v.Rows = h.s.recentEvents(events.Filter{Area: v.Area, Kind: v.Kind, Limit: 1000})
	v.Integrity, _ = events.Verify(h.s.opts.Engine.Home())
	for _, e := range v.Rows {
		v.Counts[e.Kind]++
	}
	h.s.render(w, http.StatusOK, "events", h.s.newView(r, "events", "Events", v))
}
