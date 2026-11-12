package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/services/apitokens"
	"github.com/simtabi/authama/src/internal/services/dataset"
	"github.com/simtabi/authama/src/internal/services/feed"
	"github.com/simtabi/authama/src/internal/services/history"
)

// APIPrefix is where the read API is served, when api.enabled is on.
const APIPrefix = "/api/v1"

// apiServer is the read API: JSON for programs holding an API token. It is a
// type of its own, as each set of handlers is, so no type grows into a
// facade.
//
// It shares nothing with the browser's sign-in: a session cookie is never
// read here, so a page in a browser cannot reach the API with the signed-in
// session, and the browser's pages never take a bearer token. Nothing here
// changes anything.
type apiServer struct{ s *Server }

// splitAPI sends /api/v1/ to the API, which signs requests in itself, and
// everything else on to the browser's chain.
func (s *Server) splitAPI(next http.Handler) http.Handler {
	api := apiServer{s}.handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != APIPrefix && !strings.HasPrefix(r.URL.Path, APIPrefix+"/") {
			next.ServeHTTP(w, r)
			return
		}
		if !s.opts.API {
			apiError(w, http.StatusNotFound, "api_off",
				"The API is off. Turn it on with `authama settings set api.enabled true`, then restart `authama serve`.")
			return
		}
		api.ServeHTTP(w, r)
	})
}

func (a apiServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+APIPrefix+"/{$}", a.index)
	mux.HandleFunc("GET "+APIPrefix, a.index)
	mux.HandleFunc("GET "+APIPrefix+"/profiles", a.profiles)
	mux.HandleFunc("GET "+APIPrefix+"/keys", a.keys)
	mux.HandleFunc("GET "+APIPrefix+"/keys/{profile}/{key}", a.key)
	mux.HandleFunc("GET "+APIPrefix+"/hosts", a.hosts)
	mux.HandleFunc("GET "+APIPrefix+"/tokens", a.tokens)
	mux.HandleFunc("GET "+APIPrefix+"/history", a.history)
	mux.HandleFunc("GET "+APIPrefix+"/history/{op}", a.op)
	mux.HandleFunc("GET "+APIPrefix+"/events", a.events)
	mux.HandleFunc("GET "+APIPrefix+"/events/stream", a.stream)
	mux.HandleFunc("GET "+APIPrefix+"/doctor", a.doctor)
	mux.HandleFunc(APIPrefix+"/", func(w http.ResponseWriter, r *http.Request) {
		apiError(w, http.StatusNotFound, "not_found", "No such API route. GET "+APIPrefix+" lists them.")
	})
	return a.signIn(mux)
}

// signIn admits a request carrying a live API token with the read scope, and
// only a read: the API changes nothing. A request with an Origin header came
// from a page in a browser, and is refused before its token is looked at.
func (a apiServer) signIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			apiError(w, http.StatusMethodNotAllowed, "read_only", "The API only reads: GET or HEAD.")
			return
		}
		if r.Header.Get("Origin") != "" {
			apiError(w, http.StatusForbidden, "browser_refused",
				"The API answers programs, not pages in a browser: a request with an Origin header is refused.")
			return
		}
		secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || strings.TrimSpace(secret) == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="authama"`)
			apiError(w, http.StatusUnauthorized, "unauthorized", "Send an API token: Authorization: Bearer ama_… (make one with `authama api token create <name>`).")
			return
		}
		if _, ok := apitokens.Check(a.s.opts.Engine.Home(), strings.TrimSpace(secret), apitokens.ScopeRead, a.s.now()); !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="authama", error="invalid_token"`)
			apiError(w, http.StatusUnauthorized, "invalid_token", "That API token is not valid: it was revoked, has expired, or never existed.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a apiServer) index(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"app":     "authama",
		"version": a.s.opts.Version,
		"api":     "v1",
		"routes": []string{
			APIPrefix + "/profiles", APIPrefix + "/keys", APIPrefix + "/keys/{profile}/{key}", APIPrefix + "/hosts",
			APIPrefix + "/tokens", APIPrefix + "/history", APIPrefix + "/history/{op}", APIPrefix + "/events",
			APIPrefix + "/events/stream", APIPrefix + "/doctor",
		},
	})
}

// data builds the dataset with only parts read, as the export does.
func (a apiServer) data(w http.ResponseWriter, o dataset.Options) (*dataset.Dataset, bool) {
	ds, err := dataset.Build(a.s.opts.Engine.Home(), o)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return nil, false
	}
	return ds, true
}

func (a apiServer) profiles(w http.ResponseWriter, r *http.Request) {
	if ds, ok := a.data(w, dataset.Options{Parts: []string{dataset.PartCore}}); ok {
		writeJSON(w, http.StatusOK, profilesBody{Profiles: ds.Profiles})
	}
}

func (a apiServer) keys(w http.ResponseWriter, r *http.Request) {
	if ds, ok := a.data(w, dataset.Options{Parts: []string{dataset.PartCore}}); ok {
		writeJSON(w, http.StatusOK, map[string]any{"keys": ds.Keys})
	}
}

func (a apiServer) key(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("profile") + "/" + r.PathValue("key")
	ds, ok := a.data(w, dataset.Options{Parts: []string{dataset.PartCore}})
	if !ok {
		return
	}
	i := slices.IndexFunc(ds.Keys, func(k dataset.Key) bool { return k.Ref == ref })
	if i < 0 {
		apiError(w, http.StatusNotFound, "not_found", "No key "+ref+".")
		return
	}
	var hosts []dataset.Host
	for _, h := range ds.Hosts {
		if h.KeyRef == ref {
			hosts = append(hosts, h)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": ds.Keys[i], "hosts": nonNilHosts(hosts)})
}

func (a apiServer) hosts(w http.ResponseWriter, r *http.Request) {
	if ds, ok := a.data(w, dataset.Options{Parts: []string{dataset.PartCore}}); ok {
		writeJSON(w, http.StatusOK, map[string]any{"hosts": ds.Hosts})
	}
}

// tokens says which auth tokens are set, where and of what kind; a token's
// secret is never read (the dataset has no field for one).
func (a apiServer) tokens(w http.ResponseWriter, r *http.Request) {
	if ds, ok := a.data(w, dataset.Options{Parts: []string{dataset.PartTokens}}); ok {
		writeJSON(w, http.StatusOK, map[string]any{"tokens": ds.Tokens})
	}
}

// history lists operations newest first, filtered as the history page is.
func (a apiServer) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := intParam(q.Get("limit"), 50, 500)
	if err != nil {
		apiError(w, http.StatusBadRequest, "bad_limit", err.Error())
		return
	}
	f := history.Filter{Since: q.Get("since"), Until: q.Get("until"), Command: q.Get("command"), Source: q.Get("source"),
		Host: q.Get("host"), Profile: q.Get("profile"), Key: q.Get("key"), Limit: limit}
	ops, err := a.s.opts.Engine.History(f)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	if ops == nil {
		ops = []history.Op{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": ops})
}

func (a apiServer) op(w http.ResponseWriter, r *http.Request) {
	ops, err := a.s.opts.Engine.History(history.Filter{Limit: 0})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	id := r.PathValue("op")
	for _, op := range ops {
		if op.OpID == id || (len(id) >= 8 && strings.HasPrefix(op.OpID, id)) {
			writeJSON(w, http.StatusOK, map[string]any{"operation": op})
			return
		}
	}
	apiError(w, http.StatusNotFound, "not_found", "No operation "+id+".")
}

// events is a page of the event feed after a cursor (none: the newest).
func (a apiServer) events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := intParam(q.Get("limit"), 100, feed.MaxLimit)
	if err != nil {
		apiError(w, http.StatusBadRequest, "bad_limit", err.Error())
		return
	}
	evs, next, err := feed.List(a.s.opts.Engine.Home(), q.Get("after"), limit)
	if errors.Is(err, feed.ErrBadCursor) {
		apiError(w, http.StatusBadRequest, "bad_cursor", err.Error())
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	var types []string
	if t := q.Get("type"); t != "" {
		types = strings.Split(t, ",")
	}
	out := []feed.Event{}
	for _, e := range evs {
		if feed.Matches(e, types) {
			out = append(out, e)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out, "next": next})
}

// StreamEvery is how often the event stream looks for new events.
var StreamEvery = 5 * time.Second

// stream sends the feed as server-sent events from a cursor (the after
// parameter, or Last-Event-ID, which a reconnecting client sends), or from
// now. Each event's SSE id is its cursor, so a client that reconnects
// resumes where it left off. It runs until the client goes or the server
// stops.
func (a apiServer) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		apiError(w, http.StatusInternalServerError, "no_stream", "streaming is not supported")
		return
	}
	cursor := r.Header.Get("Last-Event-ID")
	if c := r.URL.Query().Get("after"); c != "" {
		cursor = c
	}
	if cursor == "" {
		cursor = feed.Now(a.s.now())
	}
	if _, _, err := feed.List(a.s.opts.Engine.Home(), cursor, 1); errors.Is(err, feed.ErrBadCursor) {
		apiError(w, http.StatusBadRequest, "bad_cursor", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	tick := time.NewTicker(StreamEvery)
	defer tick.Stop()
	quiet := 0
	for {
		evs, next, err := feed.List(a.s.opts.Engine.Home(), cursor, feed.MaxLimit)
		if err == nil {
			for _, e := range evs {
				b, _ := json.Marshal(e)
				_, _ = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", e.Cursor(), e.Type, b)
			}
			cursor = next
			if len(evs) > 0 {
				quiet = 0
			}
		}
		// A comment every few ticks keeps proxies from closing a quiet stream.
		if quiet++; quiet%3 == 0 {
			_, _ = fmt.Fprint(w, ": still here\n\n")
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

func (a apiServer) doctor(w http.ResponseWriter, r *http.Request) {
	rep, err := a.s.opts.Engine.Doctor()
	if err != nil {
		apiError(w, http.StatusInternalServerError, "read_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": rep.OK(), "report": rep.Format(), "details": rep})
}

// profilesBody is /profiles' answer.
type profilesBody struct {
	Profiles []dataset.Profile `json:"profiles"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func apiError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func intParam(v string, def, maxN int) (int, error) {
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxN {
		return 0, fmt.Errorf("limit must be a number from 1 to %d", maxN)
	}
	return n, nil
}

func nonNilHosts(h []dataset.Host) []dataset.Host {
	if h == nil {
		return []dataset.Host{}
	}
	return h
}
