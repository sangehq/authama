package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/apitokens"
	"github.com/sangehq/authama/src/internal/services/webhooks"
)

// routePattern finds every GET route as routes.go and api.go register it:
// mux.HandleFunc("GET /keys", ...) and mux.HandleFunc("GET "+APIPrefix+"/keys", ...).
var routePattern = regexp.MustCompile(`Handle(?:Func)?\("GET (/[^"]*)"|Handle(?:Func)?\("GET "\+APIPrefix(?:\+"([^"]*)")?`)

// getRoutes is every GET route the server registers, read from its source, so
// a route added later is swept without anyone remembering to list it.
func getRoutes(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, f := range []string{"routes.go", "api.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range routePattern.FindAllStringSubmatch(string(b), -1) {
			switch {
			case m[1] != "":
				out = append(out, m[1])
			default:
				out = append(out, APIPrefix+m[2])
			}
		}
	}
	return out
}

// fill turns a route pattern into a path to request, with each wildcard set
// to something the fixture home has, so the page renders its real content.
func fill(route, opID string) string {
	r := strings.NewReplacer("{$}", "", "{profile}", "work", "{key}", "work_gh-ed25519", "{alias}", "gh",
		"{op}", opID, "{id}", "x", "{name}", "tokens.csv", "{path...}", "app.css", "{hash}", "x")
	return r.Replace(route)
}

func canary() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// No response carries a secret: not the token files authama manages, not a
// webhook's signing secret, not a read API token. Every GET route is requested,
// signed in, the read API with a token, and each body is searched for each
// planted value. Until 2026-11-14 only private keys were checked
// (TestNoPageShowsKeyMaterial); this was a one-off sweep in a review (R-GAP-01).
func TestNoResponseCarriesASecret(t *testing.T) {
	p, opID := fixtureHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	fileSecret := canary()
	for name, body := range map[string]string{
		".git-credentials":    "https://bob:" + fileSecret + "@github.com\n",
		".netrc":              "machine example.com login bob password " + fileSecret + "\n",
		".npmrc":              "//registry.npmjs.org/:_authToken=" + fileSecret + "\n",
		".composer/auth.json": `{"github-oauth":{"github.com":"` + fileSecret + `"}}`,
		".docker/config.json": `{"auths":{"ghcr.io":{"auth":"` + fileSecret + `"}}}`,
	} {
		path := filepath.Join(home, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	hook, err := webhooks.Add(p, "hook", "https://hooks.example.com/x", nil, false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	apiSecret, _, err := apitokens.Create(p, "sweep", 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	secrets := map[string]string{"a token file's secret": fileSecret, "a webhook secret": hook.Secret, "an API token": apiSecret}

	// The control: a value the pages do show must be found, or the sweep
	// cannot see bodies at all and would pass on anything.
	shown := canary()
	b, err := os.ReadFile(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	withHost := strings.Replace(string(b), `"hosts":[`, `"hosts":[{"alias":"probe","hostname":"`+shown+`.example","user":"u"},`, 1)
	if withHost == string(b) {
		t.Fatal("could not add the control host to the fixture manifest")
	}
	if err := os.WriteFile(p.Manifest(), []byte(withHost), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(Options{Engine: AppEngine{P: p}, Token: NewToken(), AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), API: true})
	if err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)

	routes := getRoutes(t)
	if len(routes) < 45 {
		t.Fatalf("found %d GET routes in routes.go and api.go; the pattern no longer matches how they are registered", len(routes))
	}
	checked, sawControl := 0, false
	for _, route := range routes {
		if strings.HasPrefix(route, "/assets/") {
			continue // the vendored public files
		}
		path := fill(route, opID)
		// Streams never end on their own; a short deadline ends each request.
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		r := httptest.NewRequest(http.MethodGet, "http://"+testHost+path, nil).WithContext(ctx)
		if strings.HasPrefix(path, APIPrefix) {
			r.Header.Set("Authorization", "Bearer "+apiSecret)
		} else {
			r.AddCookie(c.cookie)
		}
		body := do(s, r).Body.String()
		cancel()
		checked++
		for what, v := range secrets {
			if strings.Contains(body, v) {
				t.Errorf("GET %s returned %s", path, what)
			}
		}
		sawControl = sawControl || strings.Contains(body, shown)
	}
	if !sawControl {
		t.Error("no page showed the control host's name: the sweep is not reading response bodies")
	}
	t.Logf("checked %d GET routes for %d planted secrets", checked, len(secrets))
}
