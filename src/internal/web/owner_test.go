package web

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/owner"
	"github.com/sangehq/authama/src/internal/util/testcred"
)

// pinServer is a test server whose home has a PIN, with a clock the test moves.
func pinServer(t *testing.T) (*Server, *time.Time) {
	t.Helper()
	if _, err := owner.MachineID(); err != nil {
		t.Skipf("no machine id: %v", err)
	}
	t.Setenv(owner.StoreEnv, owner.StoreFile)
	p, _ := fixtureHome(t)
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	if _, err := owner.Init(p, now); err != nil {
		t.Fatal(err)
	}
	if err := owner.SetPIN(p, "", testcred.Secret("owner"), now); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Engine: AppEngine{P: p}, Token: testToken,
		AllowedHosts: AllowedHosts("127.0.0.1", 7822, nil), Version: "test",
		Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	return s, &now
}

// The sign-in link is not enough when a PIN is set: every page waits behind
// /unlock, which takes the PIN and sends the browser on to where it was going.
func TestAPINIsAskedAfterTheSignInLink(t *testing.T) {
	s, _ := pinServer(t)
	c := signIn(t, s)
	for _, path := range []string{"/", "/keys", "/tokens"} {
		rec := c.get(path)
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/unlock?next=") {
			t.Fatalf("%s before the PIN: %d %s", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := c.change("/changes/preview", addHost("x")); rec.Code != http.StatusForbidden {
		t.Fatalf("a change before the PIN: %d", rec.Code)
	}
	if body := c.get("/unlock?next=/keys").Body.String(); !strings.Contains(body, "Unlock authama") || strings.Contains(body, `href="/keys"`) {
		t.Fatalf("the unlock page is missing, or shows the navigation:\n%.400s", body)
	}
	if rec := c.change("/unlock", url.Values{"pin": {testcred.Secret("wrong")}, "next": {"/keys"}}); rec.Code != http.StatusForbidden {
		t.Fatalf("a wrong PIN: %d", rec.Code)
	}
	rec := c.change("/unlock", url.Values{"pin": {testcred.Secret("owner")}, "next": {"/keys"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/keys" {
		t.Fatalf("the right PIN: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if rec := c.get("/keys"); rec.Code != http.StatusOK {
		t.Fatalf("after the PIN: %d", rec.Code)
	}
	// The next field cannot send the browser off the server.
	c2 := signIn(t, s)
	rec = c2.change("/unlock", url.Values{"pin": {testcred.Secret("owner")}, "next": {"//evil.example/"}})
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("an off-site next: %s", loc)
	}
}

// A change confirmed within five minutes of the PIN goes through; after that
// the confirmation shows a PIN field, and is refused without the PIN.
func TestAChangeAsksForThePINAgainAfterFiveMinutes(t *testing.T) {
	s, now := pinServer(t)
	c := signIn(t, s)
	c.change("/unlock", url.Values{"pin": {testcred.Secret("owner")}})
	apply := func(alias, pin string) (int, string) {
		rec := c.change("/changes/preview", addHost(alias))
		m := hashField.FindStringSubmatch(rec.Body.String())
		if m == nil {
			t.Fatalf("no preview for %s:\n%.600s", alias, rec.Body.String())
		}
		f := addHost(alias)
		f.Set("hash", m[1])
		if pin != "" {
			f.Set("pin", pin)
		}
		rec = c.change("/changes/apply", f)
		return rec.Code, rec.Body.String()
	}
	if code, body := apply("first", ""); code != http.StatusOK {
		t.Fatalf("within the window: %d\n%.400s", code, body)
	}
	*now = now.Add(stepUpWindow + time.Second)
	if body := c.change("/changes/preview", addHost("second")).Body.String(); !strings.Contains(body, `name="pin"`) {
		t.Fatal("the confirmation shows no PIN field once the window has passed")
	}
	if code, _ := apply("second", ""); code != http.StatusForbidden {
		t.Fatalf("no PIN after the window: %d", code)
	}
	if code, _ := apply("second", testcred.Secret("wrong")); code != http.StatusForbidden {
		t.Fatalf("a wrong PIN after the window: %d", code)
	}
	if code, body := apply("second", testcred.Secret("owner")); code != http.StatusOK {
		t.Fatalf("the right PIN after the window: %d\n%.400s", code, body)
	}
}

// Without a PIN nothing changes: no unlock page, no PIN field.
func TestWithoutAPINThereIsNoUnlock(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	if rec := c.get("/"); rec.Code != http.StatusOK {
		t.Fatalf("overview: %d", rec.Code)
	}
	if body := c.change("/changes/preview", addHost("x")).Body.String(); strings.Contains(body, `name="pin"`) {
		t.Fatal("a PIN field with no PIN set")
	}
}

// A home whose owner.json was deleted keeps the PIN gate up: every page waits
// at /unlock, and the unlock page refuses with the way out rather than
// letting the browser in (review R-SEC-02).
func TestADeletedOwnerConfigKeepsThePINGateUp(t *testing.T) {
	s, _ := pinServer(t)
	if err := os.Remove(owner.ConfigPath(s.opts.Engine.Home())); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	if rec := c.get("/keys"); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/unlock") {
		t.Fatalf("/keys with owner.json gone: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec := c.change("/unlock", url.Values{"pin": {testcred.Secret("owner")}, "next": {"/keys"}})
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("unlocked a home whose owner.json is gone")
	}
	if body := rec.Body.String(); !strings.Contains(body, "owner forget") {
		t.Errorf("the refusal does not name the way out:\n%.400s", body)
	}
}
