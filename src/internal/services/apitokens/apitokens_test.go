package apitokens

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/util/paths"
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

// A revoke that lands while Check is between reading the file and writing
// LastUsed back must stay revoked: Check used to save the list it had read,
// which put the revoked token back while the CLI reported success (audit S-02).
func TestARevokeDuringCheckIsNotUndone(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	secret, _, err := Create(p, "leaked", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := Create(p, "kept", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	prev := beforeLastUsedWrite
	revoked := false
	beforeLastUsedWrite = func() {
		if !revoked {
			revoked = true
			if err := Revoke(p, "leaked"); err != nil {
				t.Errorf("revoke during check: %v", err)
			}
		}
	}
	t.Cleanup(func() { beforeLastUsedWrite = prev })

	// The request that was already inside Check is let through, but the
	// token does not come back.
	_, _ = Check(p, secret, ScopeRead, now.Add(2*time.Minute))
	if !revoked {
		t.Fatal("the interleaving was not exercised: Check wrote nothing back")
	}
	toks, err := List(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(toks) != 1 || toks[0].Name != "kept" {
		t.Fatalf("after the revoke the store holds %+v, want only \"kept\"", toks)
	}
	if _, ok := Check(p, secret, ScopeRead, now.Add(3*time.Minute)); ok {
		t.Error("the revoked token is accepted again")
	}
	if _, ok := Check(p, other, ScopeRead, now.Add(3*time.Minute)); !ok {
		t.Error("the other token was lost")
	}
}

// A token made while Check is writing LastUsed is not dropped by that write.
func TestACreateDuringCheckIsKept(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	secret, _, err := Create(p, "a", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	prev := beforeLastUsedWrite
	done := false
	beforeLastUsedWrite = func() {
		if !done {
			done = true
			if _, _, err := Create(p, "b", 0, now); err != nil {
				t.Errorf("create during check: %v", err)
			}
		}
	}
	t.Cleanup(func() { beforeLastUsedWrite = prev })
	if _, ok := Check(p, secret, ScopeRead, now.Add(2*time.Minute)); !ok {
		t.Fatal("a live token was refused")
	}
	toks, _ := List(p)
	if len(toks) != 2 {
		t.Fatalf("store holds %d tokens, want 2: %+v", len(toks), toks)
	}
	if toks[0].LastUsed.IsZero() {
		t.Error("LastUsed was not recorded")
	}
}
