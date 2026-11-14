package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/util/envnames"
	"github.com/sangehq/authama/src/internal/web"
)

// serveHandoff is what a restarting server passes the one replacing it: the
// sign-in token, so a signed-in page signs back in with the address it has;
// the port, so the address stays the same; whether to keep the home's
// record; and the old instance, to wait until it stops answering before
// listening. It travels in an owner-only file named by one environment
// variable; the new server reads and removes the file, and clears the
// variable, as soon as it starts.
type serveHandoff struct {
	// Token was the old server's first printed sign-in token.
	//
	// Deprecated: since the 2026-11-13 review (R-SEC-05) a restart mints a
	// fresh token (serveTokens) and an unused old one travels in Links, so
	// nothing reads this; it is no longer written either. Kept so a handoff
	// written by an older build still decodes. Remove no earlier than v0.2.0.
	Token string `json:"token,omitempty"`
	// Control is the key for new sign-in links, kept so the record stays valid.
	Control  string `json:"control,omitempty"`
	Port     int    `json:"port"`
	Record   bool   `json:"record"`
	Addr     string `json:"addr"` // host:port the old one listens on
	BasePath string `json:"base_path"`
	Instance string `json:"instance"`
	// Sessions are the signed-in sessions, so open pages stay signed in.
	Sessions []web.SessionState `json:"sessions,omitempty"`
	// Links are sign-in links given out and not yet used, so they still work.
	Links []web.LinkState `json:"links,omitempty"`
}

// takeHandoff reads and clears the handoff, or nil when this server is not
// replacing another.
func takeHandoff() *serveHandoff {
	v, ok := os.LookupEnv(envnames.ServeHandoff)
	if !ok {
		return nil
	}
	_ = os.Unsetenv(envnames.ServeHandoff)
	// This build passes a file's name; an older one passed the JSON itself.
	if path, ok := strings.CutPrefix(v, handoffFilePrefix); ok {
		b, err := os.ReadFile(path) //nolint:gosec // the file our own predecessor wrote
		_ = os.Remove(path)
		if err != nil {
			return nil
		}
		v = string(b)
	}
	var h serveHandoff
	// A handoff carries a control key (this build) or a token (an older
	// one); one with neither, or no port, is not a handoff.
	if json.Unmarshal([]byte(v), &h) != nil || (h.Control == "" && h.Token == "") || h.Port <= 0 {
		return nil
	}
	return &h
}

// handoffFilePrefix marks a handoff passed as a file rather than inline.
const handoffFilePrefix = "file:"

// handoffEnv writes h to an owner-only temporary file and returns the
// environment entry naming it. The handoff holds the control key, the
// sessions and their form tokens; inline in the environment they stayed
// readable in /proc/<pid>/environ for the new server's whole life (review
// R-SEC-21). The new server reads the file once and removes it.
func handoffEnv(h serveHandoff) (string, error) {
	b, err := json.Marshal(h)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "authama-handoff-*.json") // created 0600
	if err != nil {
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return envnames.ServeHandoff + "=" + handoffFilePrefix + f.Name(), nil
}

// waitGone waits until the old server stops answering as itself, so the
// port is free; at most 20 seconds.
func (h *serveHandoff) waitGone() {
	old := &serveRecord{Addr: h.Addr, BasePath: h.BasePath, Instance: h.Instance}
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(150 * time.Millisecond) {
		if !old.checkAlive() {
			return
		}
	}
}

// relaunch is the Restart a server offers its pages: start this command
// again from the binary now at this executable's path, with a handoff, then
// stop this server. The new one inherits the terminal, so its address is
// printed where this one's was.
func relaunch(h serveHandoff, state func(*serveHandoff), stop func()) func() error {
	return func() error {
		if state != nil {
			state(&h)
		}
		exe, err := ownRestartExecutable()
		if err != nil {
			return err
		}
		// The same command line, as `serve` (bare `authama` serves too, but
		// takes no serve flags), without --restart (the old server is the one
		// being replaced, not one to stop) and without opening a browser: the
		// page that asked reloads itself.
		args := slices.DeleteFunc(slices.Clone(os.Args[1:]), func(a string) bool { return a == "--restart" })
		if !slices.Contains(args, "serve") {
			args = append([]string{"serve"}, args...)
		}
		if !slices.Contains(args, "--no-open") {
			args = append(args, "--no-open")
		}
		entry, err := handoffEnv(h)
		if err != nil {
			return err
		}
		cmd := exec.Command(exe, args...) //nolint:gosec // this program's own binary, relaunched with its own arguments
		cmd.Env = append(os.Environ(), entry)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Process.Release()
		stop()
		return nil
	}
}

// foundServer is an authama web UI answering on this machine.
type foundServer struct {
	Addr     string
	Instance string
	PID      int // 0 when it is an older build that does not say
	Version  string
}

// probeClient asks a port whether authama answers there: a refused
// connection returns at once, so probing the default range is quick.
var probeClient = &http.Client{Timeout: 700 * time.Millisecond}

// findServers asks /healthz on host at each port, under basePath and at the
// root, and lists every authama that answers.
func findServers(host string, ports []int, basePath string) []foundServer {
	var out []foundServer
	seen := map[string]bool{}
	for _, port := range ports {
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		for _, base := range slices.Compact([]string{basePath, ""}) {
			resp, err := probeClient.Get("http://" + addr + base + "/healthz")
			if err != nil {
				break // nothing listens on this port
			}
			var h struct {
				App      string `json:"app"`
				Instance string `json:"instance"`
				PID      int    `json:"pid"`
				Version  string `json:"version"`
			}
			ok := resp.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&h) == nil && h.App == "authama"
			_ = resp.Body.Close()
			if ok && !seen[h.Instance] {
				seen[h.Instance] = true
				out = append(out, foundServer{Addr: addr, Instance: h.Instance, PID: h.PID, Version: h.Version})
				break
			}
		}
	}
	return out
}

// stopFound stops a found server as `serve --stop` stops the recorded one,
// and waits for it to stop answering.
func stopFound(f foundServer, basePath string) error {
	if f.PID <= 0 {
		return fmt.Errorf("it is an older build that does not say which process it is; stop it where it runs, or: lsof -ti tcp:%s -sTCP:LISTEN | xargs kill -INT", portOf(f.Addr))
	}
	if f.PID == os.Getpid() {
		return errors.New("it is this process")
	}
	if ok, name := isAuthamaProcess(f.PID); !ok {
		return fmt.Errorf("pid %d is not an authama process (%s); nothing was signalled - stop the web UI at %s where it runs", f.PID, name, f.Addr)
	}
	r := &serveRecord{PID: f.PID, Addr: f.Addr, BasePath: basePath, Instance: f.Instance}
	if !r.checkAlive() {
		r.BasePath = ""
	}
	return r.stop()
}

func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return port
}

// serverPorts are the ports `serve --stop-all` probes: the default range, a
// configured port, and the recorded server's.
func serverPorts(configured int, rec *serveRecord) []int {
	ports := []int{}
	for i := range web.DefaultPortAttempts {
		ports = append(ports, web.DefaultPort+i)
	}
	if configured > 0 && !slices.Contains(ports, configured) {
		ports = append(ports, configured)
	}
	if rec != nil {
		if n, err := strconv.Atoi(portOf(rec.Addr)); err == nil && !slices.Contains(ports, n) {
			ports = append(ports, n)
		}
	}
	return ports
}

// stopAll stops every authama web UI found, says what it did, and returns
// how many it could not stop.
// A server that does not say its pid (one from before /healthz did) is
// still stopped when the home's record names it: the record's instance
// matches the one /healthz answers, and the record holds its pid.
func stopAll(out io.Writer, host string, ports []int, basePath string, rec *serveRecord) int {
	found := findServers(host, ports, basePath)
	for i := range found {
		if found[i].PID == 0 && rec != nil && rec.PID > 0 && rec.Instance == found[i].Instance {
			found[i].PID = rec.PID
		}
	}
	if len(found) == 0 {
		_, _ = fmt.Fprintf(out, "no %s web UI answers on %s ports %d-%d\n", brand.Command, host, ports[0], ports[min(len(ports), web.DefaultPortAttempts)-1])
		return 0
	}
	failed := 0
	for _, f := range found {
		version := f.Version
		if version == "" {
			version = "an older build"
		}
		if err := stopFound(f, basePath); err != nil {
			failed++
			_, _ = fmt.Fprintf(out, "could not stop the web UI at %s (%s): %v\n", f.Addr, version, err)
			continue
		}
		_, _ = fmt.Fprintf(out, "stopped the web UI at %s (%s, pid %d)\n", f.Addr, version, f.PID)
	}
	return failed
}

// recordInstance is the recorded server's instance, or "" for none.
func recordInstance(r *serveRecord) string {
	if r == nil {
		return ""
	}
	return r.Instance
}

// handoffLinks are the sign-in links a restarted server was handed, or none.
func handoffLinks(h *serveHandoff) []web.LinkState {
	if h == nil {
		return nil
	}
	return h.Links
}

// handoffSessions are the sessions a restarted server was handed, or none.
func handoffSessions(h *serveHandoff) []web.SessionState {
	if h == nil {
		return nil
	}
	return h.Sessions
}

// serveTokens is a launch's first sign-in token and its control key. Both are
// fresh, except that a restarted server keeps the control key so the record a
// later launch reads stays valid. The old printed token is never reused: if
// it was unused it is still among the handoff's Links, and if it was used,
// re-arming it would let the address in a browser's history sign in again.
func serveTokens(h *serveHandoff) (token, control string) {
	token, control = web.NewToken(), web.NewToken()
	if h != nil && h.Control != "" {
		control = h.Control
	}
	return token, control
}

// restartExecutable is the binary a restart runs: exe, this process's own
// path, while a file is there; otherwise the authama lookPath resolves, for a
// server started from a checkout that has since moved (macOS keeps reporting
// the old path); otherwise the reason there is nothing to run.
func restartExecutable(exe string, lookPath func(string) (string, error)) (string, error) {
	if fi, err := os.Stat(exe); err == nil && !fi.IsDir() {
		return exe, nil
	}
	found, err := lookPath(brand.Command)
	if err != nil {
		return "", fmt.Errorf("the binary this server started from (%s) is gone, and no %s is on PATH: start the web UI again from where authama is now", exe, brand.Command)
	}
	if real, err := filepath.EvalSymlinks(found); err == nil {
		found = real
	}
	return found, nil
}

// ownRestartExecutable is restartExecutable for this process.
func ownRestartExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return restartExecutable(exe, exec.LookPath)
}
