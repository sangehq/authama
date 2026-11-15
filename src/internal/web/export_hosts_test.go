package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/encode"
)

// /exports/run writes hosts in each host format: ssh_config and termius are
// the old downloads' bytes, every client format downloads under the client's
// own file name, and the preview says which hosts a format leaves out.
func TestHostFormatsDownloadFromTheExportPipeline(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	for run, old := range map[string]string{
		"/exports/run?preset=hosts&format=ssh_config":            "/exports/ssh_config",
		"/exports/run?preset=hosts&format=termius":               "/exports/termius-hosts.csv",
		"/exports/run?preset=hosts&format=termius&include_git=1": "/exports/termius-hosts.csv?include_git=1",
	} {
		got, want := c.get(run), c.get(old)
		if got.Code != http.StatusOK || want.Code != http.StatusOK || got.Body.String() != want.Body.String() {
			t.Errorf("%s (%d) differs from %s (%d):\n%s\n---\n%s", run, got.Code, old, want.Code, got.Body.String(), want.Body.String())
		}
	}
	checked := 0
	for _, n := range encode.HostNames() {
		rec := c.get("/exports/run?preset=hosts&include_git=1&format=" + n)
		f, _ := encode.Get(n)
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 || rec.Header().Get("Content-Type") != f.MediaType {
			t.Errorf("%s: %d %q %s", n, rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="`) || strings.Contains(cd, "/") {
			t.Errorf("%s: Content-Disposition %q", n, cd)
		}
		checked++
	}
	if checked < 9 {
		t.Fatalf("checked %d host formats", checked)
	}
	if rec := c.get("/exports/run?preset=hosts&format=putty"); !strings.Contains(rec.Header().Get("Content-Disposition"), "authama-putty.reg") {
		t.Errorf("putty's file name: %q", rec.Header().Get("Content-Disposition"))
	}

	// The only host is git, which a client format leaves out without include_git.
	var p exportPreview
	if err := json.Unmarshal(c.get("/exports/run?preset=hosts&preview=1&format=putty").Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Skipped) != 1 || !strings.HasPrefix(p.Skipped[0], "gh: a git host") {
		t.Errorf("skipped = %v", p.Skipped)
	}
	// A host format of another list is refused, and not offered there.
	if rec := c.get("/exports/run?preset=keys&format=putty"); rec.Code != http.StatusBadRequest {
		t.Errorf("putty of keys: %d", rec.Code)
	}
	var kp exportPreview
	if err := json.Unmarshal(c.get("/exports/run?preset=keys&preview=1").Body.Bytes(), &kp); err != nil {
		t.Fatal(err)
	}
	for _, f := range kp.Formats {
		if f.Hosts {
			t.Errorf("keys offer the host format %s", f.Name)
		}
	}
}
