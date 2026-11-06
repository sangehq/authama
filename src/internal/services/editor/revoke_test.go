package editor

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/util/testcred"
)

const revokeJSON = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
  "work":{"key_scope":"per_service","keys":[{"name":"shared-ed25519"}],"hosts":[
    {"alias":"gh","hostname":"github.com","user":"git","key_name":"shared-ed25519","provider":"github","token_env":"AUTHAMA_REVOKE_TEST_TOKEN"},
    {"alias":"cb","hostname":"codeberg.org","user":"git","key_name":"shared-ed25519","provider":"codeberg"}]}}}`

// --revoke used to be a stub that revoked nothing while the help said it did.
// Deleting a host with --revoke now asks the host's provider to remove the
// key, as rotation does: GitHub through gh, here a stand-in on PATH that
// lists the key and accepts the delete. A provider that cannot remove keys
// (Codeberg's web panel) is reported as not revoked, never as revoked.
func TestRevokeAsksTheProviderAndReportsWhatItCouldNot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the gh stand-in is a shell script")
	}
	when := "2026-01-01"
	ed, p := writeManifest(t, revokeJSON, map[string]inventory.KeyRecord{
		"SHA256:shared": {
			Profile: "work", Path: "~/.ssh/profiles/work/shared-ed25519", Type: "ed25519",
			Deployments: []inventory.Deployment{
				{Target: "gh", Method: "github", Date: &when, Verified: true},
				{Target: "cb", Method: "web-panel", Date: &when, Verified: true},
			},
		},
	})
	body := base64.StdEncoding.EncodeToString([]byte("\x00\x00\x00\x0bssh-ed25519" + strings.Repeat("k", 32)))
	pub := "ssh-ed25519 " + body + " work/gh"
	dir := filepath.Join(p.SSHDir, "profiles", "work")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "shared-ed25519.pub"), []byte(pub+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	calls := filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + calls + "'\n" +
		"case \"$*\" in\n" +
		"  *\"--method DELETE\"*) exit 0 ;;\n" +
		"  *\"user/keys\"*) printf '%s\\n' '[{\"id\": 42, \"key\": \"ssh-ed25519 " + body + "\"}]'; exit 0 ;;\n" +
		"esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AUTHAMA_REVOKE_TEST_TOKEN", testcred.Secret("revoke"))

	res, err := ed.DeleteHost("work", "gh", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Revoked) != 1 || res.Revoked[0] != "gh" {
		t.Errorf("revoked = %v, want [gh]", res.Revoked)
	}
	log, _ := os.ReadFile(calls)
	if !strings.Contains(string(log), "--method DELETE user/keys/42") {
		t.Errorf("gh was not asked to delete the key:\n%s", log)
	}

	res, err = ed.DeleteHost("work", "cb", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Revoked) != 0 || len(res.NotRevoked) != 1 || res.NotRevoked[0] != "cb" {
		t.Errorf("codeberg: revoked %v, not revoked %v; want it reported as not revoked", res.Revoked, res.NotRevoked)
	}
}
