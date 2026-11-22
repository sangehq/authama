package web

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sangehq/authama/src/internal/services/settings"
	"github.com/sangehq/authama/src/internal/util/applog"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/util/tz"
)

// statusWriter remembers the status a handler wrote, for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Flush keeps streamed answers (a job's live output) streaming.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests writes each request to authama.log: a change (any method but
// GET or HEAD) at info, a page view at debug. Only the path is written,
// never the query, so a sign-in link's token cannot reach the log; the log
// redacts one anyway.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		attrs := []any{"area", "web", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "duration_ms", time.Since(start).Milliseconds(), "remote", r.RemoteAddr}
		switch {
		case r.Method != http.MethodGet && r.Method != http.MethodHead:
			applog.Info("request", attrs...)
		case sw.status >= 500:
			applog.Warn("request", attrs...)
		default:
			applog.Debug("request", attrs...)
		}
	})
}

// zoneKey is the setting that names the zone pages show times in.
const zoneKey = "display.timezone"

// followZone shows times in display.timezone as it is now: a change made on
// the Settings page (or by the CLI while the server runs) applies from the
// next page, without a restart, and taking the setting out goes back to this
// system's zone. The settings file is read only when its modification time
// moves. A zone from the environment wins over the file, as it does at
// start-up, and so does --tz: then nothing is followed.
func followZone(file string, next http.Handler) http.Handler {
	var mu sync.Mutex
	var seen time.Time
	env := os.Getenv(envnames.Timezone)
	fv, inFile := settings.FileValue(file, zoneKey)
	fromEnv := env != "" && (!inFile || fv != env)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fromEnv || tz.Pinned() {
			next.ServeHTTP(w, r)
			return
		}
		if fi, err := os.Stat(file); err == nil {
			mu.Lock()
			if !fi.ModTime().Equal(seen) {
				seen = fi.ModTime()
				if v, ok := settings.FileValue(file, zoneKey); ok {
					_ = tz.Set(v) // a bad value keeps the zone in use; the setting's check refuses one anyway
				} else {
					_ = tz.Set("")
				}
			}
			mu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}
