package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/owner"
	"github.com/sangehq/authama/src/internal/services/webhooks"
	"github.com/sangehq/authama/src/internal/util/envnames"
)

// A home whose seal was removed is refused, with what to do, by `owner
// verify`, `doctor` and `serve` starting (audit A-SEC-08); serve never binds
// a port for it. The way back, `owner forget`, still works. The OS-store case
// is proven in services/owner with a fake store; here the files say it, so no
// test reads the real keychain.
func TestASealRemovedIsRefusedAtVerifyDoctorAndServe(t *testing.T) {
	p := editHome(t)
	t.Setenv(owner.StoreEnv, owner.StoreFile)
	run(t, "owner", "init", "--yes")
	if err := os.Remove(owner.ConfigPath(p)); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"owner", "verify"}, {"doctor"}} {
		out, err := runErr(t, args...)
		if err == nil {
			t.Errorf("%v passed a home whose seal was removed:\n%s", args, out)
			continue
		}
		if all := out + err.Error(); !strings.Contains(all, "seal was removed") || !strings.Contains(all, "owner forget") {
			t.Errorf("%v does not say the seal was removed and what to do:\n%s\n%v", args, out, err)
		}
	}
	tried, stop := fakeServe(t)
	out, err := runServe(t, stop)
	if err == nil || !strings.Contains(err.Error(), "seal was removed") {
		t.Fatalf("serve started on a home whose seal was removed: %v\n%s", err, out)
	}
	if len(*tried) != 0 {
		t.Errorf("serve bound %v before refusing", *tried)
	}
	run(t, "owner", "forget", "--yes")
	if _, err := runErr(t, "owner", "verify"); err != nil {
		t.Errorf("owner forget did not clear the state: %v", err)
	}
}

// webhooks.include_details chooses the full payload; unset, the redacted one
// (audit A-SEC-12).
func TestWebhooksIncludeDetailsChoosesTheFullPayload(t *testing.T) {
	editHome(t)
	if got := webhooksDetail(); got != webhooks.DetailRedacted {
		t.Errorf("by default: %v, want redacted", got)
	}
	t.Setenv(envnames.WebhooksIncludeDetails, "1")
	if got := webhooksDetail(); got != webhooks.DetailFull {
		t.Errorf("with webhooks.include_details on: %v, want full", got)
	}
}
