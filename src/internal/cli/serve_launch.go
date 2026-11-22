package cli

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

// Every launch - bare authama, authama serve, a double-click - checks the
// server this home recorded before it opens it, and puts right what it finds:
//
//   - a record whose server is gone is removed;
//   - a server that holds its port but no longer answers (hung) is stopped,
//     once its pid is shown to be authama's web UI;
//   - a server from an older build than this one, or one whose binary on disk
//     is this one but rebuilt, is replaced by this build on the same port;
//   - a server on another port than web.port names, when that port is free,
//     or under another web.base_path, is replaced to match.
//
// Then the browser is opened with a fresh one-time sign-in link.

// launchProbe asks a server's /healthz with a timeout of its own, to tell a
// hung server (connects, never answers) from one that is gone.
var launchProbe = &http.Client{Timeout: 2 * time.Second}

// serveHung reports whether r's server holds its address but does not answer:
// the port accepts a connection and /healthz times out. A server that answers
// at all - even as another instance - is not hung, so another home's server
// that took the port is never touched. Swappable in tests.
var serveHung = func(r *serveRecord) bool {
	conn, err := net.DialTimeout("tcp", r.Addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	resp, err := launchProbe.Get("http://" + r.Addr + r.BasePath + "/healthz")
	if err == nil {
		_ = resp.Body.Close()
		return false
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// serveReap stops a hung server's process: an interrupt, then a kill after
// five seconds. Only a pid that is shown to run authama's web UI is signalled.
// Swappable in tests.
var serveReap = func(r *serveRecord) error {
	if ok, why := isAuthamaProcess(r.PID); !ok {
		return fmt.Errorf("pid %d is not authama (%s); nothing was stopped", r.PID, why)
	}
	if ok, args := isServingProcess(r.PID); !ok {
		return fmt.Errorf("pid %d is not authama's web UI (%s); nothing was stopped", r.PID, args)
	}
	proc, err := os.FindProcess(r.PID)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = proc.Signal(os.Interrupt)
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
			if !portTaken(r.Addr) {
				return nil
			}
		}
	}
	return proc.Kill()
}

// serveStop stops a recorded server that a launch replaces. Swappable in
// tests, where the recorded pid is made up.
var serveStop = func(r *serveRecord) error { return r.stop() }

// portTaken reports whether something accepts connections on addr.
func portTaken(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// launchFix is what a launch found wrong with the running server, and so
// replaces it: empty when the server is fine to open as it is.
type launchFix struct {
	Why  string
	Port int // the port to start the replacement on
}

// replaceReason decides whether the recorded server, alive and proven, is
// replaced rather than opened. ours is this build's version and exe this
// binary; configuredPort is web.port when it is set (0 when not), and
// basePath the path this launch would serve under.
func replaceReason(r *serveRecord, ours, exe string, configuredPort int, basePath string, theirExe string) launchFix {
	_, portStr, _ := net.SplitHostPort(r.Addr)
	port, _ := strconv.Atoi(portStr)
	if r.Version != ours {
		if exe != "" && theirExe != "" && sameFile(exe, theirExe) {
			return launchFix{Why: fmt.Sprintf("its binary was rebuilt (it runs %s, the file is now %s)", r.Version, ours), Port: port}
		}
		if c, ok := compareVersions(ours, r.Version); ok && c > 0 {
			return launchFix{Why: fmt.Sprintf("it runs %s, an older build than this %s", r.Version, ours), Port: port}
		}
	}
	if r.BasePath != basePath {
		return launchFix{Why: fmt.Sprintf("it serves under %q, and web.base_path is now %q", r.BasePath+"/", basePath+"/"), Port: port}
	}
	if configuredPort != 0 && configuredPort != port && !portTaken(net.JoinHostPort("127.0.0.1", strconv.Itoa(configuredPort))) {
		return launchFix{Why: fmt.Sprintf("it listens on %d, and web.port is %d", port, configuredPort), Port: configuredPort}
	}
	return launchFix{}
}

// sameFile reports whether two paths name the same file, symlinks resolved.
func sameFile(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	if a == b {
		return true
	}
	fa, errA := os.Stat(a)
	fb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(fa, fb)
}

var versionRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-(\d+)-g[0-9a-f]+)?(?:-dirty)?$`)

// compareVersions orders two builds by `git describe` version: the release,
// then the commits since it. ok is false when either is not such a version
// (a "dev" build), and then neither is said to be newer.
func compareVersions(a, b string) (cmp int, ok bool) {
	pa, okA := versionParts(a)
	pb, okB := versionParts(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range pa {
		switch {
		case pa[i] > pb[i]:
			return 1, true
		case pa[i] < pb[i]:
			return -1, true
		}
	}
	return 0, true
}

func versionParts(v string) ([4]int, bool) {
	var out [4]int
	m := versionRe.FindStringSubmatch(v)
	if m == nil {
		return out, false
	}
	for i := 0; i < 4; i++ {
		if m[i+1] != "" {
			out[i], _ = strconv.Atoi(m[i+1])
		}
	}
	return out, true
}

// canOpenBrowser reports whether this session has a desktop to open a browser
// on: not over SSH, and on Linux and the BSDs only with a display. why says
// what stopped it.
func canOpenBrowser(goos string, getenv func(string) string) (ok bool, why string) {
	for _, v := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		if getenv(v) != "" {
			return false, "this is an SSH session"
		}
	}
	if goos != "darwin" && goos != "windows" && getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
		return false, "there is no desktop session (DISPLAY and WAYLAND_DISPLAY are not set)"
	}
	return true, ""
}
