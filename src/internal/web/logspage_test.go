package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/applog"
	"github.com/sangehq/authama/src/internal/util/tz"
)

func logTo(t *testing.T, s *Server) string {
	t.Helper()
	dir := s.opts.Engine.Home().LogDir()
	applog.Configure(applog.Config{Dir: dir, Level: applog.ParseLevel("debug")})
	t.Cleanup(func() { applog.Configure(applog.Config{}) })
	return dir
}

// The Logs page lists records newest first, filters by level and text,
// follows new ones, and downloads them; a sign-in link's token never reaches
// the log.
func TestTheLogsPageFiltersFollowsAndDownloads(t *testing.T) {
	s, _, _ := newTestServer(t)
	dir := logTo(t, s)
	c := signIn(t, s)
	applog.Warn("rotation failed", "area", "job", "command", "ssh key rotate")
	applog.Info("command", "area", "cli", "command", "ssh host list")

	page := c.get("/logs").Body.String()
	for _, want := range []string{"rotation failed", "ssh host list", "signin.link", "data-log-follow"} {
		if !strings.Contains(page, want) {
			t.Errorf("Logs page lacks %q", want)
		}
	}
	if strings.Index(page, "ssh host list") > strings.Index(page, "rotation failed") {
		t.Error("the Logs page is not newest first")
	}
	warn := c.get("/logs?level=warn").Body.String()
	if !strings.Contains(warn, "rotation failed") || strings.Contains(warn, "ssh host list") {
		t.Error("level=warn shows the wrong records")
	}
	grep := c.get("/logs?grep=HOST+LIST").Body.String()
	if !strings.Contains(grep, "ssh host list") || strings.Contains(grep, "rotation failed") {
		t.Error("grep shows the wrong records")
	}
	if rec := c.get("/logs?since=yesterdayish"); rec.Code != http.StatusBadRequest {
		t.Errorf("a bad since = %d", rec.Code)
	}

	off := applog.Size(dir)
	applog.Info("later on")
	rec := c.get("/logs/tail?offset=" + itoa(off))
	var tail struct {
		Offset int64    `json:"offset"`
		Rows   []LogRow `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tail); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	found := false
	for _, r := range tail.Rows {
		found = found || r.Msg == "later on"
	}
	if !found || tail.Offset <= off {
		t.Fatalf("tail: %+v", tail)
	}

	dl := c.get("/logs/download?level=warn")
	if !strings.Contains(dl.Header().Get("Content-Disposition"), "attachment") || !strings.Contains(dl.Body.String(), "rotation failed") ||
		strings.Contains(dl.Body.String(), "ssh host list") {
		t.Fatalf("download: %v %s", dl.Header(), dl.Body.String())
	}

	b, _ := os.ReadFile(dir + "/" + applog.FileName)
	if strings.Contains(string(b), testToken) {
		t.Fatal("the log holds the server's sign-in token")
	}
}

// Without a session the Logs page, its feed and its download answer 401.
func TestTheLogsNeedASession(t *testing.T) {
	s, _, _ := newTestServer(t)
	for _, path := range []string{"/logs", "/logs/tail", "/logs/download"} {
		if rec := do(s, httptest.NewRequest(http.MethodGet, "http://"+testHost+path, nil)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d", path, rec.Code)
		}
	}
}

// The Settings page's time zone field searches the IANA zones and shows the
// time there; pages show times in the zone in use.
func TestTheTimeZoneSettingIsASearchableField(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	page := c.get("/settings").Body.String()
	for _, want := range []string{`list="tz-zones"`, `<option value="Europe/Berlin">`, `data-tz-preview="tz-now"`, "authama settings timezone set"} {
		if !strings.Contains(page, want) {
			t.Errorf("Settings lacks %q", want)
		}
	}
	t.Cleanup(func() { _ = tz.Set("") })
	if err := tz.Set("Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.get("/logs").Body.String(), "Asia/Tokyo") {
		t.Error("the Logs page does not name the zone its times are in")
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// A zone set or taken out while the server runs applies from the next page.
func TestTheServerFollowsTheTimeZoneSetting(t *testing.T) {
	t.Setenv("AUTHAMA_TIMEZONE", "")
	t.Cleanup(func() { _ = tz.Set("") })
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if err := os.WriteFile(p.SettingsFile(), []byte(`{"display": {"timezone": "Asia/Tokyo"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c.get("/logs")
	if tz.Name() != "Asia/Tokyo" {
		t.Fatalf("zone after the setting = %q", tz.Name())
	}
	if err := os.WriteFile(p.SettingsFile(), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(p.SettingsFile(), future, future)
	c.get("/logs")
	if tz.Name() != "" {
		t.Fatalf("zone after taking the setting out = %q, want this system's", tz.Name())
	}
}
