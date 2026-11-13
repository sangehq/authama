package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/paths"
	"github.com/simtabi/authama/src/internal/web"
)

// serveRecord is what a running `authama serve` leaves in its home's state
// directory, so the next `authama serve` for the same home opens it instead
// of starting another. Without it every launch started a new server and none
// ever stopped: twenty of them filled ports 7822-7841 and the next launch
// failed. The file is owner-only because URL carries the sign-in token, which
// the server already prints to the same user.
type serveRecord struct {
	PID      int       `json:"pid"`
	Addr     string    `json:"addr"` // host:port, as bound
	URL      string    `json:"url"`  // the sign-in address, token included
	Instance string    `json:"instance"`
	ReadOnly bool      `json:"read_only"`
	Version  string    `json:"version"`
	Started  time.Time `json:"started"`
	// BasePath is the path it serves under (web.base_path), "" at the root.
	BasePath string `json:"base_path,omitempty"`
	// Control is the key that gets a new sign-in link from the server
	// (web.LinkPath). The record is mode 0600 in the state home, so only its
	// owner reads it. A record from before this field has none.
	Control string `json:"control,omitempty"`
}

// address is the server's address with no sign-in token.
func (r *serveRecord) address() string { return "http://" + r.Addr + r.BasePath + "/" }

// newLink asks the recorded server for a new sign-in address, which works once
// within web.LinkLifetime: the one it printed when it started may be used or
// expired.
func (r *serveRecord) newLink() (string, error) {
	if r.Control == "" {
		return "", errors.New("its record has no control key (a server from an older build)")
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+r.Addr+r.BasePath+web.LinkPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set(web.ControlHeader, r.Control)
	resp, err := healthClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var reply struct {
		SignIn string `json:"sign_in"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&reply) != nil || !strings.HasPrefix(reply.SignIn, "?token=") {
		return "", fmt.Errorf("it answered %d", resp.StatusCode)
	}
	return r.address() + reply.SignIn, nil
}

func serveRecordPath(p paths.Paths) string { return filepath.Join(p.StateDir(), "serve.json") }

// readServeRecord returns the record, or nil when there is none or it does not
// parse (a record from a crashed server is just stale, never an error).
func readServeRecord(p paths.Paths) *serveRecord {
	b, err := os.ReadFile(serveRecordPath(p))
	if err != nil {
		return nil
	}
	var r serveRecord
	if json.Unmarshal(b, &r) != nil || r.Addr == "" || r.Instance == "" {
		return nil
	}
	return &r
}

func writeServeRecord(p paths.Paths, r serveRecord) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := fs.EnsureDir(p.StateDir(), 0o700); err != nil {
		return err
	}
	return fs.WriteTextAtomic(serveRecordPath(p), string(b)+"\n", 0o600)
}

// removeServeRecord removes the record if it is still this instance's, so a
// server stopping late never deletes the record of the one that replaced it.
func removeServeRecord(p paths.Paths, instance string) {
	if r := readServeRecord(p); r != nil && r.Instance == instance {
		_ = os.Remove(serveRecordPath(p))
	}
}

// healthClient asks a recorded server whether it is still there. Short: a
// server that does not answer in a second is treated as gone.
var healthClient = &http.Client{Timeout: time.Second}

// checkAlive reports whether the server r names still answers, as that instance.
// The instance id, not the pid or the port, is what proves it: a pid can be
// reused and a port can be taken by something else once the server is gone.
func (r *serveRecord) checkAlive() bool {
	resp, err := healthClient.Get("http://" + r.Addr + r.BasePath + "/healthz")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	var h struct {
		App      string `json:"app"`
		Instance string `json:"instance"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&h) != nil {
		return false
	}
	return h.App == "authama" && h.Instance == r.Instance
}

// stop asks the recorded server to shut down - an interrupt where the OS has
// one, so it closes its listener and record as Ctrl-C would - and waits for it
// to stop answering.
func (r *serveRecord) stop() error {
	proc, err := os.FindProcess(r.PID)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		err = proc.Kill()
	} else {
		err = proc.Signal(os.Interrupt)
	}
	if err != nil {
		return err
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if !serveAlive(r) {
			return nil
		}
	}
	return errors.New("the server did not stop within 10 seconds")
}

// serveNewLink is newLink, swappable in tests, for the same reason as
// serveAlive.
var serveNewLink = func(r *serveRecord) (string, error) { return r.newLink() }

// serveAlive is checkAlive, swappable in tests, which serve on a fake
// listener that no HTTP request can reach.
var serveAlive = func(r *serveRecord) bool { return r.checkAlive() }
