package web

import (
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/mutation"
)

// fakeRemotes is git with origins in a map and no network.
type fakeRemotes struct {
	origins map[string]string
	refuse  map[string]bool // URLs ls-remote refuses
	proved  []string
}

func (f *fakeRemotes) Origin(dir string) (string, error) {
	if u, ok := f.origins[dir]; ok {
		return u, nil
	}
	return "", errors.New("no origin")
}

func (f *fakeRemotes) LsRemote(_, u string) error {
	f.proved = append(f.proved, u)
	if f.refuse[u] {
		return errors.New("permission denied")
	}
	return nil
}

func (f *fakeRemotes) SetOrigin(dir, u string) error { f.origins[dir] = u; return nil }

// /git-sign shows each checkout's remote mode, as `ssh git status` does, and
// switches them through the preview, as `ssh git use` does: the preview
// changes nothing, and apply proves each new URL before it rewrites origin.
func TestTheGitSignPageSwitchesRemoteModes(t *testing.T) {
	sandboxGitConfig(t)
	// Real absolute paths, so they are absolute on Windows too (a drive).
	w := t.TempDir()
	app, lib, none := filepath.Join(w, "app"), filepath.Join(w, "lib"), filepath.Join(w, "none")
	fake := &fakeRemotes{origins: map[string]string{app: "git@github.com:acme/app.git", lib: "https://gitlab.com/x/y.git"},
		refuse: map[string]bool{}}
	old := change.RemoteGit
	change.RemoteGit = fake
	t.Cleanup(func() { change.RemoteGit = old })
	s, _, _ := newTestServer(t)
	t.Cleanup(mutation.Release)
	c := signIn(t, s)

	body := c.get("/git-sign?remotes=" + url.QueryEscape(app+"\n"+lib+"\n"+none)).Body.String()
	for _, want := range []string{"acme/app", "not a managed GitHub remote", "no origin", `value="git-remote-use"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("status misses %q:\n%.3000s", want, body)
		}
	}
	if b := c.get("/git-sign?remotes=relative/path").Body.String(); !strings.Contains(b, "give a checkout&#39;s full path") {
		t.Fatal("a relative path was read")
	}

	f := url.Values{"action": {"git-remote-use"}, "mode": {"https"}, "profile": {"work"}, "checkouts": {app + "\n" + lib}}
	prev := c.change("/changes/preview", f)
	pb := prev.Body.String()
	if prev.Code != http.StatusOK || !strings.Contains(pb, "https://github.com/acme/app.git") || !strings.Contains(pb, "skipped") {
		t.Fatalf("preview: %d\n%.2500s", prev.Code, pb)
	}
	if fake.origins[app] != "git@github.com:acme/app.git" || len(fake.proved) != 0 {
		t.Fatal("the preview changed a remote or reached the network")
	}
	hash := hashField.FindStringSubmatch(pb)
	if hash == nil {
		t.Fatalf("no confirm hash:\n%.2000s", pb)
	}
	f.Set("hash", hash[1])
	if rec := c.change("/changes/apply", f); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Made the change") {
		t.Fatalf("apply: %d\n%.1500s", rec.Code, rec.Body.String())
	}
	if fake.origins[app] != "https://github.com/acme/app.git" || len(fake.proved) != 1 {
		t.Fatalf("after apply: %v, proved %v", fake.origins, fake.proved)
	}
	if bad := c.change("/changes/preview", url.Values{"action": {"git-remote-use"}, "mode": {"ftp"}, "checkouts": {app}}); bad.Code == http.StatusOK && hashField.MatchString(bad.Body.String()) {
		t.Fatal("an unknown mode was offered for confirmation")
	}
}
