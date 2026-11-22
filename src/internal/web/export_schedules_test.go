package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/savedsel"
)

// The Export page's Schedules card adds, runs and removes export schedules
// as `export schedule add|run|remove` do, and refuses a folder in ~/.ssh.
func TestExportSchedulesFromTheWeb(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if body := c.get("/export").Body.String(); !strings.Contains(body, `id="export-schedules"`) || !strings.Contains(body, "Save a selection above first") {
		t.Fatalf("/export without a saved selection:\n%.200s", body)
	}
	if rec := c.postJSON("/api/export/selections", `{"name":"prd","selection":`+prdHosts+`}`); rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	folder := filepath.Join(t.TempDir(), "reports")
	if rec := c.change("/export/schedules/add", url.Values{"selection": {"prd"}, "format": {"json"}, "folder": {filepath.Join(p.SSHDir, "out")}}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "refusing") {
		t.Fatalf("a folder in ~/.ssh: %d", rec.Code)
	}
	if rec := c.change("/export/schedules/add", url.Values{"selection": {"prd"}, "format": {"json"}, "folder": {"reports"}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("a relative folder: %d", rec.Code)
	}
	if rec := c.change("/export/schedules/add", url.Values{"selection": {"nope"}, "folder": {folder}}); rec.Code != http.StatusBadRequest {
		t.Fatalf("an unknown selection: %d", rec.Code)
	}
	rec := c.change("/export/schedules/add", url.Values{"selection": {"prd"}, "format": {"json"}, "folder": {folder}})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Scheduled prd daily") {
		t.Fatalf("add: %d\n%.400s", rec.Code, rec.Body.String())
	}
	if all, _ := savedsel.Schedules(p); len(all) != 1 || all[0].Folder != folder {
		t.Fatalf("schedules: %+v", all)
	}
	rec = c.change("/export/schedules/run", url.Values{})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "row(s) to") {
		t.Fatalf("run: %d", rec.Code)
	}
	if fi, err := os.Stat(filepath.Join(folder, "prd.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the file written: %v %v", fi, err)
	}
	if rec := c.change("/export/schedules/remove", url.Values{"selection": {"prd"}}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Removed 1") {
		t.Fatalf("remove: %d", rec.Code)
	}
}
