package apitokens

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/util/paths"
)

// A token is shown once and kept only as its hash; it is accepted for its
// scope until it expires or is revoked, and a near miss is refused.
func TestATokenIsCheckedByItsHashUntilItGoes(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	secret, rec, err := Create(p, "ci", 30, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, Prefix) || len(secret) < 40 {
		t.Fatalf("token %q is not an authama token", secret)
	}
	b, _ := os.ReadFile(File(p))
	if strings.Contains(string(b), secret) || strings.Contains(string(b), strings.TrimPrefix(secret, Prefix)) {
		t.Fatal("the file holds the token itself")
	}
	// Windows has no mode bits: its permissions are ACLs (util/perms).
	if fi, _ := os.Stat(File(p)); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v, not owner-only", fi.Mode().Perm())
	}
	if _, ok := Check(p, secret, ScopeRead, now); !ok {
		t.Fatal("a fresh token is refused")
	}
	if _, ok := Check(p, secret, "write", now); ok {
		t.Error("a read token is accepted for another scope")
	}
	if _, ok := Check(p, secret[:len(secret)-1]+"x", ScopeRead, now); ok {
		t.Error("a near miss is accepted")
	}
	if _, ok := Check(p, strings.TrimPrefix(secret, Prefix), ScopeRead, now); ok {
		t.Error("a token without its prefix is accepted")
	}
	if _, ok := Check(p, secret, ScopeRead, rec.Expires); ok {
		t.Error("an expired token is accepted")
	}
	if _, _, err := Create(p, "ci", 0, now); err == nil {
		t.Error("a second token by the same name was made")
	}
	if _, _, err := Create(p, "Bad Name", 0, now); err == nil {
		t.Error("a name with spaces and capitals was accepted")
	}
	if err := Revoke(p, "ci"); err != nil {
		t.Fatal(err)
	}
	if _, ok := Check(p, secret, ScopeRead, now); ok {
		t.Error("a revoked token is accepted")
	}
	if err := Revoke(p, "ci"); err == nil {
		t.Error("revoking a token twice did not say it was gone")
	}
}
