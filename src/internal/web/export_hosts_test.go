package web

import (
	"encoding/json"
	"net/http"
	"os"
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

// ?termius_groups= nests the Termius download's Groups as --termius-groups
// does, on the old download and the pipeline alike. ?profile= still chooses
// rows by profile, since a nested group is not a profile name, and the rows
// it keeps carry their nested group; a scheme that is not one is a 400.
func TestTermiusDownloadNestsGroups(t *testing.T) {
	s, p, _ := newTestServer(t)
	if err := os.WriteFile(p.Manifest(), []byte(`{"version":1,"profiles":{
		"work":{"code":"wrk","category":"clients","hosts":[{"alias":"wrk-prd-web-00","hostname":"203.0.113.5","user":"deploy"}]},
		"solo":{"hosts":[{"alias":"box","hostname":"box.example","user":"me"}]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	old := c.get("/exports/termius-hosts.csv?termius_groups=category/profile/env")
	if old.Code != http.StatusOK || !strings.Contains(old.Body.String(), "\nclients/work/prd,wrk-prd-web-00,") ||
		!strings.Contains(old.Body.String(), "\nsolo,box,") {
		t.Fatalf("nested download (%d):\n%s", old.Code, old.Body.String())
	}
	if run := c.get("/exports/run?preset=hosts&format=termius&termius_groups=category/profile/env"); run.Body.String() != old.Body.String() {
		t.Errorf("the pipeline differs from the download:\n%s\n---\n%s", run.Body.String(), old.Body.String())
	}
	if plain := c.get("/exports/termius-hosts.csv").Body.String(); !strings.Contains(plain, "\nwork,wrk-prd-web-00,") {
		t.Errorf("the default groups by profile alone:\n%s", plain)
	}
	shaped := c.get("/exports/termius-hosts.csv?termius_groups=profile/env&profile=work").Body.String()
	if !strings.Contains(shaped, "work/prd,wrk-prd-web-00,") || strings.Contains(shaped, "box") {
		t.Errorf("?profile=work with nested groups:\n%s", shaped)
	}
	if rec := c.get("/exports/termius-hosts.csv?termius_groups=env"); rec.Code != http.StatusBadRequest {
		t.Errorf("an unknown scheme: %d", rec.Code)
	}
	if page := c.get("/hosts").Body.String(); !strings.Contains(page, "/exports/termius-hosts.csv?termius_groups=profile/env") &&
		!strings.Contains(page, "/exports/termius-hosts.csv?termius_groups=profile%2fenv") {
		t.Error("the hosts page offers no nested Termius download")
	}
}
