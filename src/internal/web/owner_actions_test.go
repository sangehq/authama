package web

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/services/owner"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

// The owner binding is made from the web like any change: bound, then a PIN
// set, previewed and confirmed. The PIN is never in either page.
func TestTheOwnerBindingIsMadeFromThePage(t *testing.T) {
	if _, err := owner.MachineID(); err != nil {
		t.Skipf("no machine id: %v", err)
	}
	t.Setenv(owner.StoreEnv, owner.StoreFile)
	s, p, _ := newTestServer(t)
	c := signIn(t, s)
	confirm := func(f url.Values) (int, string) {
		rec := c.change("/changes/preview", f)
		m := hashField.FindStringSubmatch(rec.Body.String())
		if m == nil {
			return rec.Code, rec.Body.String()
		}
		g := url.Values{}
		for k, v := range f {
			if !strings.HasPrefix(k, "pin_") {
				g[k] = v
			}
		}
		// The confirmation carries the stash references, never the PIN.
		for _, ref := range refField.FindAllStringSubmatch(rec.Body.String(), -1) {
			g.Set(ref[1], ref[2])
		}
		g.Set("hash", m[1])
		rec = c.change("/changes/apply", g)
		return rec.Code, rec.Body.String()
	}
	if code, body := confirm(url.Values{"action": {"owner-init"}}); code != http.StatusOK || !strings.Contains(body, "Made the change") {
		t.Fatalf("init: %d\n%.500s", code, body)
	}
	pin := testcred.Secret("pin")
	preview := c.change("/changes/preview", url.Values{"action": {"owner-pin-set"}, "pin_new": {pin}, "pin_confirm": {pin}}).Body.String()
	if strings.Contains(preview, pin) || !strings.Contains(preview, "owner pin reset") {
		t.Fatalf("the preview holds the PIN or omits the reset:\n%.800s", preview)
	}
	code, body := confirm(url.Values{"action": {"owner-pin-set"}, "pin_new": {pin}, "pin_confirm": {pin}})
	if code != http.StatusOK || strings.Contains(body, pin) {
		t.Fatalf("pin set: %d\n%.500s", code, body)
	}
	if err := owner.CheckPIN(p, pin, time.Now()); err != nil {
		t.Fatalf("the PIN set from the page does not check: %v", err)
	}
}

// A PIN that could not be reset if forgotten is refused at preview: the
// guard against locking yourself out.
func TestAPINThatCouldNotBeResetIsRefused(t *testing.T) {
	if _, err := owner.MachineID(); err != nil {
		t.Skipf("no machine id: %v", err)
	}
	t.Setenv(owner.StoreEnv, owner.StoreFile)
	s, p, _ := newTestServer(t)
	if _, err := owner.Init(p, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(p.StateDir(), "owner.key")); err != nil {
		t.Fatal(err)
	}
	c := signIn(t, s)
	pin := testcred.Secret("pin")
	body := c.change("/changes/preview", url.Values{"action": {"owner-pin-set"}, "pin_new": {pin}, "pin_confirm": {pin}}).Body.String()
	if hashField.MatchString(body) || !strings.Contains(body, "could not be reset") {
		t.Fatalf("a PIN with no way back was offered for confirmation:\n%.600s", body)
	}
	if c, _ := owner.Load(p); c.HasPIN() {
		t.Fatal("a PIN was set")
	}
}

var refField = regexp.MustCompile(`name="(pin_[a-z]+_ref)" value="([^"]+)"`)
