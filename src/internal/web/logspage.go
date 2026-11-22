package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/recency"
	"github.com/sangehq/authama/src/internal/util/applog"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// logsPage is the Logs page: authama.log, filtered, searched, followed and
// downloaded. Every record was redacted when it was written.
type logsPage struct{ s *Server }

// LogRow is one record as the page shows it.
type LogRow struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Attrs string `json:"attrs"`
}

// LogsView is the Logs page.
type LogsView struct {
	Rows                 []LogRow
	Level, Since, Grep   string
	Levels               []string
	Path, SettingLevel   string
	Retention, MaxSizeMB string
	Offset               int64
	Limit                int
	Zone                 string
	Truncated            bool
}

const logsPageLimit = 500

// logFilter reads the page's filters; a bad --since is named, not guessed.
func logFilter(q url.Values, now time.Time) (applog.Filter, error) {
	f := applog.Filter{Level: applog.ParseLevel("debug"), Grep: strings.TrimSpace(q.Get("grep"))}
	if l := q.Get("level"); l != "" && applog.ValidLevel(l) {
		f.Level = applog.ParseLevel(l)
	}
	if s := strings.TrimSpace(q.Get("since")); s != "" {
		t, err := recency.ParseSince(s, now)
		if err != nil {
			return f, err
		}
		f.Since = t
	}
	return f, nil
}

func logRow(e applog.Entry) LogRow {
	keys := make([]string, 0, len(e.Attrs))
	for k := range e.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		v := e.Attrs[k]
		if s, ok := v.(string); ok {
			fmt.Fprintf(&b, "%s=%s", k, s)
			continue
		}
		raw, _ := json.Marshal(v)
		fmt.Fprintf(&b, "%s=%s", k, raw)
	}
	return LogRow{Time: tz.Format(e.Time, "2006-01-02 15:04:05 MST"), Level: strings.ToLower(e.Level), Msg: e.Msg, Attrs: b.String()}
}

func (h logsPage) dir() string { return h.s.opts.Engine.Home().LogDir() }

// handle answers GET /logs.
func (h logsPage) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v := LogsView{Level: q.Get("level"), Since: q.Get("since"), Grep: q.Get("grep"), Levels: applog.Levels,
		Path: filepath.Join(h.dir(), applog.FileName), Limit: logsPageLimit, Zone: tz.Describe(),
		SettingLevel: orDefault(envnames.Get(nil, envnames.LogLevel), applog.DefaultLevel),
		Retention:    orDefault(envnames.Get(nil, envnames.LogRetentionDays), strconv.Itoa(applog.DefaultRetentionDays)),
		MaxSizeMB:    orDefault(envnames.Get(nil, envnames.LogMaxSizeMB), strconv.Itoa(applog.DefaultMaxSizeMB))}
	f, err := logFilter(q, h.s.now())
	if err != nil {
		h.s.renderError(w, r, http.StatusBadRequest, "Bad filter", err.Error())
		return
	}
	f.Limit = logsPageLimit + 1
	entries, err := applog.Read(h.dir(), f)
	if err != nil {
		h.s.renderError(w, r, http.StatusInternalServerError, "Could not read the log", err.Error())
		return
	}
	if len(entries) > logsPageLimit {
		entries, v.Truncated = entries[1:], true
	}
	for i := len(entries) - 1; i >= 0; i-- {
		v.Rows = append(v.Rows, logRow(entries[i]))
	}
	v.Offset = applog.Size(h.dir())
	h.s.render(w, http.StatusOK, "logs", h.s.newView(r, "logs", "Logs", v))
}

// handleTail answers GET /logs/tail?offset=N: the records written since, for
// the page's live follow.
func (h logsPage) handleTail(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f, err := logFilter(q, h.s.now())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)
	if offset < 0 {
		offset = 0
	}
	entries, next, err := applog.Tail(h.dir(), offset, f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]LogRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, logRow(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"offset": next, "rows": rows})
}

// handleDownload answers GET /logs/download: the filtered records as JSON
// lines, as `authama logs export` writes them.
func (h logsPage) handleDownload(w http.ResponseWriter, r *http.Request) {
	f, err := logFilter(r.URL.Query(), h.s.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := applog.Read(h.dir(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"authama-log-%s.jsonl\"", h.s.now().UTC().Format("20060102-150405")))
	w.Header().Set("Cache-Control", "no-store")
	for _, e := range entries {
		_, _ = fmt.Fprintln(w, e.Raw)
	}
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
