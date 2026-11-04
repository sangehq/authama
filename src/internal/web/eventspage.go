package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/history"
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
	_, _ = events.Scan(p, s.opts.EventRoots(p), s.now().UTC())
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
	v := EventsView{Area: strings.TrimSpace(q.Get("area")), Kind: strings.TrimSpace(q.Get("kind")),
		Tracking: h.s.opts.EventRoots != nil, Counts: map[string]int{}}
	switch v.Area {
	case "", events.AreaSSH, events.AreaConfig, events.AreaAuth:
	default:
		v.Area = ""
	}
	switch v.Kind {
	case "", events.KindAdded, events.KindChanged, events.KindRemoved, events.KindMode, events.KindStarted:
	default:
		v.Kind = ""
	}
	v.Rows = h.s.recentEvents(events.Filter{Area: v.Area, Kind: v.Kind, Limit: 1000})
	for _, e := range v.Rows {
		v.Counts[e.Kind]++
	}
	h.s.render(w, http.StatusOK, "events", h.s.newView(r, "events", "Events", v))
}
