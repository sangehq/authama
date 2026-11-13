// Package web is the `authama serve` command's localhost web UI.
//
// Server-rendered html/template pages over the same services the CLI calls,
// with a little vanilla JavaScript and Tabler's CSS, all embedded in
// the binary: the page makes no network request beyond this server. It never
// imports internal/cli, which holds process-wide state a long-lived server
// cannot share (imports_test.go holds it to that).
//
// Reading takes no lock. A change takes it only while it is checked and made
// (see services/change), never while a person reads its preview, so a running
// server does not hold up a CLI command.
package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/simtabi/authama/src/internal/services/events"
	"github.com/simtabi/authama/src/internal/services/jobs"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Options configures a Server.
type Options struct {
	// Engine reads the authama home. Required.
	Engine Engine

	// Token is the first sign-in link's token, printed in the URL. It works
	// once, for LinkLifetime. Required.
	Token string
	// Control is the key `authama serve` presents at LinkPath for a new
	// sign-in link. Empty: no link is made that way.
	Control string
	// Links are sign-in links a restarted server gave out and nobody used yet.
	Links []LinkState

	// AllowedHosts are the Host header values answered (see AllowedHosts).
	AllowedHosts []string

	// AllowRemote records that the bind is not loopback; it lets a TLS
	// reverse proxy's X-Forwarded-Proto mark the session cookie Secure.
	AllowRemote bool

	// ReadOnly registers no route that changes anything, and hides the forms.
	ReadOnly bool

	// API serves the read API at /api/v1/ for API tokens (api.enabled).
	API bool

	// Version and BindAddr are shown in the footer.
	Version  string
	BindAddr string

	// BasePath is the path the UI is served under (web.base_path), as
	// CleanBasePath gives it: "" for the root, else "/authama". Every link,
	// form, asset, redirect and the session cookie carry it.
	BasePath string
	// HideExport leaves Export out of the menu (web.menu_export false); the
	// page itself stays reachable, as each list's download does.
	HideExport bool

	// Settings reads the settings page for a home. The values in effect were
	// settled at start-up, which the command that started the server owns,
	// so it passes this in. Nil shows the files only, with no values.
	Settings func(paths.Paths) SettingsView

	// EventRoots names the files whose changes the events page records:
	// ~/.ssh, the config home, the tools' credential files. Nil tracks
	// nothing. The server scans them every EventInterval while it runs, and
	// when the events page or the overview opens.
	EventRoots    func(paths.Paths) []events.Tracked
	EventInterval time.Duration

	// Retain is how many snapshots a snapshot restore keeps when it snapshots
	// first, as AUTHAMA_SNAPSHOT_RETAIN sets for the CLI. 0 means 10.
	Retain int

	// Now is the clock. Nil means time.Now.
	Now func() time.Time

	// Instance is a random id /healthz answers with, so `authama serve` can
	// tell a running server it started from another process holding the port
	// (see cli's serve record). Empty answers with none.
	Instance string
	// Restart relaunches this server from the binary now installed at its
	// path, on the same address and with the same sign-in token, and stops
	// this one; nil when the server cannot (a test, an embedded use). The
	// stale-build banner offers it as "Restart now".
	Restart func() error
	// AutoRestart restarts the server by itself when its binary is rebuilt
	// or reinstalled (web.auto_restart, on by default), and every open page
	// reloads into the new build. Off, the page says it is older and offers
	// "Restart now".
	AutoRestart bool
	// Sessions are signed-in sessions a restarting server handed over.
	Sessions []SessionState
}

// Server is the web UI.
type Server struct {
	opts     Options
	pages    *pageSet
	assets   *assetSet
	sessions sessions
	links    links
	secrets  secretStash
	who      whoami
	jobs     *jobs.Registry
	handler  http.Handler
	binary   binaryWatch
}

// New builds a Server. It parses every template once, so a template error is
// a startup error rather than a 500 on first visit.
func New(opts Options) (*Server, error) {
	if opts.Engine == nil {
		return nil, errors.New("web: an Engine is required")
	}
	if opts.Token == "" {
		return nil, errors.New("web: a Token is required")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	base, err := CleanBasePath(opts.BasePath)
	if err != nil {
		return nil, err
	}
	opts.BasePath = base
	assets, err := loadAssets()
	if err != nil {
		return nil, err
	}
	pages, err := parsePages(assets, base)
	if err != nil {
		return nil, err
	}
	s := &Server{opts: opts, pages: pages, assets: assets, jobs: jobs.NewRegistry(), who: currentUser(), binary: watchOwnBinary(opts.Now())}
	s.sessions.adopt(opts.Sessions, opts.Now())
	s.links.add(opts.Token, opts.Now().Add(LinkLifetime))
	for _, l := range opts.Links {
		if l.Token != "" {
			s.links.add(l.Token, l.Expires)
		}
	}
	s.handler = securityHeaders(withBasePath(base, s.hostCheck(s.splitAPI(s.auth(s.sameOrigin(ownerGate{s}.requireUnlock(s.routes())))))))
	return s, nil
}

// Handler returns the full middleware chain and routes.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) now() time.Time { return s.opts.Now() }

// BinaryCheckEvery is how often a server with AutoRestart looks at its binary.
var BinaryCheckEvery = 2 * time.Second

// Serve answers on ln until ctx is cancelled, then shuts down with a
// ten-second grace for requests in flight.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		// A body (a form) is read in 30 seconds or not at all. There is no
		// WriteTimeout: a job's event stream stays open while the job runs.
		ReadTimeout:    30 * time.Second,
		IdleTimeout:    2 * time.Minute,
		MaxHeaderBytes: 64 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	// The event watcher stops with the server and is waited for, so no scan
	// is still writing the home after Serve returns.
	watchCtx, stopWatch := context.WithCancel(ctx)
	watched := make(chan struct{})
	if s.opts.EventRoots != nil {
		go func() { defer close(watched); s.watchEvents(watchCtx) }()
	} else {
		close(watched)
	}
	defer func() { stopWatch(); <-watched }()
	// Restart by itself when the binary is rebuilt (web.auto_restart).
	go s.watchBinary(watchCtx.Done(), BinaryCheckEvery)
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

type ctxKey int

const sessionKey ctxKey = 0

func withSession(ctx context.Context, s *session) context.Context {
	return context.WithValue(ctx, sessionKey, s)
}

func sessionFrom(ctx context.Context) *session {
	s, _ := ctx.Value(sessionKey).(*session)
	return s
}
