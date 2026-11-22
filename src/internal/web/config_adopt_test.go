package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// `ssh config adopt` runs from /ssh-config like any change: previewed as a
// diff, confirmed, and it takes out only the hand-written block the managed
// one reproduces.
func TestConfigAdoptFromThePage(t *testing.T) {
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	if body := c.get("/ssh-config").Body.String(); !strings.Contains(body, `value="config-adopt"`) {
		t.Fatal("/ssh-config offers no adopt button")
	}
	apply := func(action string) string {
		t.Helper()
		f := url.Values{"action": {action}}
		rec := c.change("/changes/preview", f)
		m := hashField.FindStringSubmatch(rec.Body.String())
		if rec.Code != http.StatusOK || m == nil {
			t.Fatalf("%s preview: %d\n%.800s", action, rec.Code, rec.Body.String())
		}
		f.Set("hash", m[1])
		rec = c.change("/changes/apply", f)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
			t.Fatalf("%s apply: %d\n%.800s", action, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	apply("config-render")
	cfg := filepath.Join(p.SSHDir, "config")
	body, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Copy the first managed Host block, line for line, above the managed block:
	// what an import before R-BUG-09 left behind.
	block := regexp.MustCompile(`(?m)^Host [^\n]+\n(?:[ \t]+[^\n]*\n)+`).Find(body)
	if block == nil {
		t.Skipf("the fixture renders no host:\n%s", body)
	}
	if err := os.WriteFile(cfg, append(append(append([]byte{}, block...), '\n'), body...), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := c.change("/changes/preview", url.Values{"action": {"config-adopt"}}).Body.String()
	if !strings.Contains(preview, "ssh config adopt --yes") {
		t.Errorf("the preview does not name the command:\n%.800s", preview)
	}
	apply("config-adopt")
	after, _ := os.ReadFile(cfg)
	if strings.Count(string(after), string(block)) != 1 {
		t.Errorf("the duplicate block is still there:\n%s", after)
	}
}
