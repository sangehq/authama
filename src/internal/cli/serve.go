package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/util/desktop"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/version"
	"github.com/sangehq/authama/src/internal/web"
)

// Seams for tests: the sandbox and CI cannot always bind a socket, and a test
// must not open a browser or wait for Ctrl-C.
var (
	serveListen  web.ListenFunc = net.Listen
	serveBrowser                = openBrowser
	serveContext                = func(parent context.Context) (context.Context, context.CancelFunc) {
		return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	}
	serveNotifications = listenForNotifications
)

// listenForNotifications lets other programs and scripts show a notification
// as authama while the web UI runs (`authama notify ping`, or go-notifykit's
// Ping). It listens only when notifications can be shown at all, so it stays
// off under go test, in CI and with AUTHAMA_NOTIFICATIONS=off, and a failure
// to listen never stops the web UI.
func listenForNotifications(ctx context.Context, errOut io.Writer) {
	n, err := desktop.NewNotifier()
	if err != nil {
		return
	}
	if ok, _ := n.Enabled(); !ok {
		return
	}
	go func() {
		if err := n.Listen(ctx); err != nil {
			_, _ = fmt.Fprintf(errOut, "notifications: not listening for pings: %v\n", err)
		}
	}()
}

func newServeCmd() *cobra.Command {
	var (
		addr        string
		port        int
		allowRemote bool
		allowHosts  []string
		readOnly    bool
		noOpen      bool
		tokenFile   string
		fresh       bool
		status      bool
		stopRunning bool
		stopEvery   bool
		restart     bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Open the web UI on this machine",
		Long: "Open the web UI on this machine: an overview, the profiles, hosts and keys, the\n" +
			"snapshots, the operation history, and the doctor and audit reports.\n\n" +
			"Changes - adding, editing and deleting profiles, hosts and keys, and re-rendering\n" +
			"the config - are previewed first: the page shows exactly what each would write,\n" +
			"and confirming makes it only if nothing moved since. The lock is held only while\n" +
			"a change is checked and made, so a running server does not hold up a command.\n" +
			"--read-only serves no change at all.\n\n" +
			"Listens on 127.0.0.1, port " + strconv.Itoa(web.DefaultPort) + " by default. If that port is taken it takes the\n" +
			"next free one and prints the address it bound; --port pins a port and fails if it\n" +
			"is taken. The printed address carries a one-time token that signs the browser in;\n" +
			"without it every page answers 401.\n\n" +
			"One server per home: when one is already running for this home, serve opens\n" +
			"it again instead of starting another, so launching twice - or double-clicking\n" +
			"the app - never piles servers up. --new starts another anyway; --status says\n" +
			"whether one is running and where; --stop stops it. --stop-all stops every\n" +
			"authama web UI answering on this machine's usual ports, whichever home or build;\n" +
			"--restart does that and starts a fresh one, which is how to load a new build.\n" +
			"A page from an older build says so, and can restart its server itself. When every port from the\n" +
			"default up is taken, any free port is used.\n\n" +
			"web.addr, web.port and web.base_path in authama.json (or AUTHAMA_WEB_ADDR,\n" +
			"AUTHAMA_WEB_PORT, AUTHAMA_WEB_BASE_PATH) change the defaults: a configured port\n" +
			"falls back to the next free one like the default, and web.base_path serves the\n" +
			"UI under a path such as /authama, for a reverse proxy. --addr and --port win.\n\n" +
			"A non-loopback --addr is refused unless --allow-remote is passed. Prefer an SSH\n" +
			"tunnel: ssh -L " + strconv.Itoa(web.DefaultPort) + ":127.0.0.1:" + strconv.Itoa(web.DefaultPort) + " <host>.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if !c.Flags().Changed("addr") {
				if v := envnames.Get(nil, envnames.WebAddr); v != "" {
					addr = v
				}
			}
			if !c.Flags().Changed("port") {
				if v := envnames.Get(nil, envnames.WebPort); v != "" {
					n, err := strconv.Atoi(v)
					if err != nil || n < 1 || n > 65535 {
						return fmt.Errorf("web.port %q: want a port from 1 to 65535", v)
					}
					port = n
				}
			}
			basePath, err := web.CleanBasePath(envnames.Get(nil, envnames.WebBasePath))
			if err != nil {
				return err
			}
			host, pinnedPort, hasPort, err := splitAddr(addr)
			if err != nil {
				return fmt.Errorf("--addr %q: %w", addr, err)
			}
			pinned := hasPort || c.Flags().Changed("port")
			if !hasPort {
				pinnedPort = port
			}
			if pinnedPort < 0 || pinnedPort > 65535 {
				return fmt.Errorf("--port %d: want 0-65535", pinnedPort)
			}
			if !web.IsLoopback(host) && !allowRemote {
				return fmt.Errorf("--addr %s is reachable from other machines; pass --allow-remote to mean it, "+
					"or tunnel instead: ssh -L %d:127.0.0.1:%d <host>", host, web.DefaultPort, web.DefaultPort)
			}

			out := c.OutOrStdout()
			running := readServeRecord(p)
			if running != nil && !serveAlive(running) {
				running = nil // a record left by a server that is gone
			}
			// A server replacing one that restarted: wait for the old one to
			// let go of its port, then take the same port. The sign-in token is
			// fresh (serveTokens); unused links come across in the handoff.
			handoff := takeHandoff()
			if handoff != nil {
				handoff.waitGone()
				running = nil
				pinnedPort = handoff.Port
			}
			if stopEvery || restart {
				if failed := stopAll(out, displayHost(host), serverPorts(port, running), basePath, running); failed > 0 && stopEvery {
					return fmt.Errorf("%d web UI(s) could not be stopped", failed)
				}
				removeServeRecord(p, recordInstance(running))
				if stopEvery {
					return nil
				}
				running = nil
			}
			if status || stopRunning {
				if running == nil {
					_, _ = fmt.Fprintf(out, "no %s web UI is running for %s\n", brand.Command, p.ConfigDir)
					return nil
				}
				if stopRunning {
					if err := running.stop(); err != nil {
						return fmt.Errorf("stopping the server (pid %d): %w", running.PID, err)
					}
					removeServeRecord(p, running.Instance)
					_, _ = fmt.Fprintf(out, "stopped the web UI at %s (pid %d)\n", running.Addr, running.PID)
					return nil
				}
				_, _ = fmt.Fprintf(out, "running since %s, pid %d: %s\n",
					running.Started.Local().Format("2006-01-02 15:04"), running.PID, running.address())
				return nil
			}
			// Reuse the running server, unless asked for a new one or for
			// something it is not: another port, read-only when it is not, or
			// a remote bind.
			if running != nil && !fresh && !pinned && !allowRemote && web.IsLoopback(host) && running.ReadOnly == readOnly &&
				running.BasePath == basePath {
				// The address it printed when it started has a link that works
				// once, so it is asked for a new one.
				link, err := serveNewLink(running)
				if err != nil {
					return fmt.Errorf("%s web UI is already running for this home (pid %d) at %s, but gave no sign-in link: %v; "+
						"restart it with `%s serve --restart`", brand.Name, running.PID, running.address(), err, brand.Command)
				}
				_, _ = fmt.Fprintf(out, "%s web UI is already running for this home (pid %d): %s\n", brand.Name, running.PID, link)
				_, _ = fmt.Fprintf(out, "the link signs in once, within %d minutes; stop the server with `%s serve --stop`, or start another with `%s serve --new`\n",
					int(web.LinkLifetime/time.Minute), brand.Command, brand.Command)
				if tokenFile != "" {
					if err := writeTokenFile(tokenFile, link); err != nil {
						return err
					}
				}
				if !noOpen && platform.StdinIsTerminal() {
					if err := serveBrowser(link); err != nil {
						_, _ = fmt.Fprintf(c.ErrOrStderr(), "could not open a browser (%s); open the address above yourself\n", err)
					}
				}
				return nil
			}

			ln, err := web.Listen(serveListen, host, pinnedPort, pinned || handoff != nil, web.DefaultPortAttempts)
			if err != nil {
				return err
			}
			bound := ln.Addr().(*net.TCPAddr).Port

			token, control := serveTokens(handoff)
			instance := web.NewToken()[:16]
			// Recorded only for the default kind of server, the one a later
			// `authama serve` would reuse: a pinned or remote one is a choice
			// the next launch should not inherit. A restarted server keeps the
			// choice the one it replaced made.
			record := !pinned && !allowRemote && web.IsLoopback(host)
			if handoff != nil {
				record = handoff.Record
			}
			var stopServing context.CancelFunc
			var srvRef *web.Server
			restartFn := relaunch(serveHandoff{Control: control, Port: bound, Record: record,
				Addr: net.JoinHostPort(displayHost(host), strconv.Itoa(bound)), BasePath: basePath, Instance: instance},
				func(h *serveHandoff) {
					if srvRef != nil {
						h.Sessions, h.Links = srvRef.Sessions(), web.PendingLinks(srvRef)
					}
				},
				func() {
					if stopServing != nil {
						stopServing()
					}
				})
			srv, err := web.New(web.Options{
				HideExport:   !settingOn(envnames.WebMenuExport, true),
				Instance:     instance,
				Engine:       web.AppEngine{P: p, Retain: snapshotRetain()},
				Settings:     settingsView,
				EventRoots:   eventRoots,
				Retain:       snapshotRetain(),
				Token:        token,
				Control:      control,
				Links:        handoffLinks(handoff),
				AllowedHosts: web.AllowedHosts(host, bound, allowHosts),
				AllowRemote:  allowRemote,
				ReadOnly:     readOnly,
				Version:      version.Version,
				BindAddr:     ln.Addr().String(),
				BasePath:     basePath,
				Restart:      restartFn,
				AutoRestart:  settingOn(envnames.WebAutoRestart, true),
				API:          settingOn(envnames.API, false),
				Sessions:     handoffSessions(handoff),
			})
			if err != nil {
				_ = ln.Close()
				return err
			}
			srvRef = srv

			url := fmt.Sprintf("http://%s%s/?token=%s", net.JoinHostPort(displayHost(host), strconv.Itoa(bound)), basePath, token)
			if record {
				if err := writeServeRecord(p, serveRecord{PID: os.Getpid(), Addr: net.JoinHostPort(displayHost(host), strconv.Itoa(bound)),
					URL: url, Instance: instance, ReadOnly: readOnly, Version: version.Version, Started: time.Now(),
					BasePath: basePath, Control: control}); err != nil {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "could not record this server (%v); the next launch will start another\n", err)
				}
				defer removeServeRecord(p, instance)
			}
			if bound != pinnedPort && pinnedPort != 0 {
				_, _ = fmt.Fprintf(out, "port %d is in use; listening on %d instead\n", pinnedPort, bound)
			}
			_, _ = fmt.Fprintf(out, "%s web UI: %s\n", brand.Name, url)
			_, _ = fmt.Fprintf(out, "the link signs in once, within %d minutes; run `%s serve` again for another\n",
				int(web.LinkLifetime/time.Minute), brand.Command)
			mode := "changes are previewed before they are made"
			if readOnly {
				mode = "read-only"
			}
			_, _ = fmt.Fprintf(out, "home %s (%s)\n", p.ConfigDir, mode)
			if settingOn(envnames.API, false) {
				_, _ = fmt.Fprintf(out, "read API on: %s%s (API tokens only: authama api token create)\n",
					strings.TrimSuffix(strings.SplitN(url, "?", 2)[0], "/"), "/api/v1")
			}
			if settingOn(envnames.Webhooks, false) {
				_, _ = fmt.Fprintln(out, "webhooks on: events are sent to the endpoints in `authama webhook list`")
			}
			if allowRemote {
				_, _ = fmt.Fprintln(c.ErrOrStderr(), "warning: --allow-remote serves plain HTTP; put a TLS proxy in front, or use an SSH tunnel")
			}
			_, _ = fmt.Fprintln(out, "press Ctrl-C to stop")

			if tokenFile != "" {
				if err := writeTokenFile(tokenFile, url); err != nil {
					_ = ln.Close()
					return err
				}
			}
			if !noOpen && platform.StdinIsTerminal() {
				if err := serveBrowser(url); err != nil {
					_, _ = fmt.Fprintf(c.ErrOrStderr(), "could not open a browser (%s); open the address above yourself\n", err)
				}
			}

			ctx, stop := serveContext(c.Context())
			defer stop()
			stopServing = stop
			serveNotifications(ctx, c.ErrOrStderr())
			serveWebhooks(ctx, p, c.ErrOrStderr())
			return srv.Serve(ctx, ln)
		},
	}
	f := cmd.Flags()
	f.StringVar(&addr, "addr", "127.0.0.1", "address to listen on, host or host:port (a port here pins it, like --port)")
	f.IntVar(&port, "port", web.DefaultPort, "port to listen on; when not given and the default is taken, the next free port is used")
	f.BoolVar(&allowRemote, "allow-remote", false, "allow a non-loopback --addr (plain HTTP; prefer an SSH tunnel)")
	f.StringArrayVar(&allowHosts, "allow-host", nil, "extra Host header to accept with --allow-remote, e.g. a proxy's name (repeatable)")
	f.BoolVar(&readOnly, "read-only", false, "serve no route that changes anything, and show no form")
	f.BoolVar(&noOpen, "no-open", false, "do not open a browser")
	f.StringVar(&tokenFile, "token-file", "", "also write the sign-in address to this file, mode 0600, for scripts")
	f.BoolVar(&fresh, "new", false, "start another server even when one is already running for this home")
	f.BoolVar(&status, "status", false, "say whether a server is running for this home, and its address")
	f.BoolVar(&stopRunning, "stop", false, "stop the server running for this home")
	f.BoolVar(&stopEvery, "stop-all", false, "stop every authama web UI answering on this machine's usual ports, whichever home or build")
	f.BoolVar(&restart, "restart", false, "stop every authama web UI found, then start a fresh one: how to load a new build")
	cmd.MarkFlagsMutuallyExclusive("new", "status", "stop", "stop-all", "restart")
	return cmd
}

// splitAddr accepts "host" or "host:port".
func splitAddr(addr string) (host string, port int, hasPort bool, err error) {
	h, p, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		if addr == "" {
			return "", 0, false, errors.New("empty address")
		}
		return addr, 0, false, nil
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, false, fmt.Errorf("port %q is not a number", p)
	}
	return h, n, true, nil
}

// displayHost is the host to print: a wildcard bind is reachable here as
// 127.0.0.1, and that is one of the Host headers the server accepts.
func displayHost(host string) string {
	switch host {
	case "", "0.0.0.0", "::":
		return "127.0.0.1"
	}
	return host
}

// writeTokenFile writes the sign-in address owner-only: it is a credential
// for as long as the server runs.
func writeTokenFile(path, url string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, url); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// O_CREATE's mode is not applied to a file that already existed.
	return os.Chmod(path, 0o600)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
