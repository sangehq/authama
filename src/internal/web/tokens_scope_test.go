package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/mutation"
)

// /tokens?project= reads a project's own files, as `token list --project`
// does, and deleting from there removes the project's credential, never the
// user's.
func TestTheTokensPageScopesToAProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)
	proj := t.TempDir()
	npmrc := filepath.Join(proj, ".npmrc")
	if err := os.WriteFile(npmrc, []byte("//npm.example.com/:_authToken=s3cr3t-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if body := c.get("/tokens").Body.String(); strings.Contains(body, "npm.example.com") {
		t.Fatal("the user view shows the project's token")
	}
	body := c.get("/tokens?project=" + url.QueryEscape(proj)).Body.String()
	if !strings.Contains(body, "npm.example.com") || !strings.Contains(body, "Showing the project at") || strings.Contains(body, "s3cr3t-value") {
		t.Fatalf("the project view:\n%.3000s", body)
	}
	if !strings.Contains(body, `name="project" value="`+proj+`"`) {
		t.Fatal("the forms do not carry the project")
	}
	if b := c.get("/tokens?project=relative").Body.String(); !strings.Contains(b, "give a full path") {
		t.Fatal("a relative project was read")
	}
	f := url.Values{"action": {"token-delete"}, "target": {"npm"}, "host": {"npm.example.com"}, "project": {proj}}
	prev := c.change("/changes/preview", f)
	pb := prev.Body.String()
	hash := hashField.FindStringSubmatch(pb)
	if prev.Code != http.StatusOK || hash == nil || !strings.Contains(pb, npmrc) {
		t.Fatalf("preview: %d\n%.2500s", prev.Code, pb)
	}
	f.Set("hash", hash[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.1500s", rec.Code, rec.Body.String())
	}
	if b, _ := os.ReadFile(npmrc); strings.Contains(string(b), "npm.example.com") {
		t.Fatalf("the project's credential is still there:\n%s", b)
	}
	// Apply takes --config, --source and --target too: a bad config path is
	// refused in the preview.
	bad := c.change("/changes/preview", url.Values{"action": {"token-apply"}, "config": {filepath.Join(proj, "none.json")}, "only_npm": {"1"}})
	if bad.Code == http.StatusOK && hashField.MatchString(bad.Body.String()) {
		t.Fatal("a missing --config was offered for confirmation")
	}
}
