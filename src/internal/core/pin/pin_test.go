package pin

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	"github.com/sangehq/authama/src/internal/util/testcred"
)

func salt() []byte { return []byte(testcred.Secret("salt"))[:SaltLen] }

func argonKey(p string, salt []byte, t, m uint32, threads uint8) []byte {
	return argon2.IDKey([]byte(p), salt, t, m, threads, keyLen)
}

func b64(b []byte) string { return base64.RawStdEncoding.EncodeToString(b) }

func TestAHashChecksTheRightPINAndNoOther(t *testing.T) {
	p := testcred.Secret("owner")
	enc, err := Hash(p, salt())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, "argon2id$v=19$m=65536,t=3,p=4$") || strings.Contains(enc, p) {
		t.Fatalf("encoded form: %s", enc)
	}
	if ok, stale, err := Verify(enc, p); !ok || stale || err != nil {
		t.Fatalf("Verify(right) = %v %v %v", ok, stale, err)
	}
	if ok, _, _ := Verify(enc, p+"x"); ok {
		t.Fatal("a different PIN verified")
	}
}

// A PIN set by an earlier release, hashed with PBKDF2 at its full cost, still
// unlocks, and is reported stale so the next correct entry rehashes it as
// Argon2id. Nobody's PIN stops working when the algorithm changes.
func TestAPBKDF2HashFromAnEarlierReleaseStillVerifiesAndIsStale(t *testing.T) {
	p := testcred.Secret("legacy")
	for _, iter := range []int{Iterations, 1000} {
		enc, err := hashPBKDF2(p, salt(), iter)
		if err != nil {
			t.Fatal(err)
		}
		if ok, stale, err := Verify(enc, p); !ok || !stale || err != nil {
			t.Fatalf("Verify(pbkdf2 at %d) = %v %v %v", iter, ok, stale, err)
		}
		if ok, _, _ := Verify(enc, p+"x"); ok {
			t.Fatalf("a different PIN verified against the pbkdf2 hash at %d", iter)
		}
	}
}

// An Argon2id hash below the current cost verifies and is stale.
func TestAWeakerArgon2idHashVerifiesAndIsStale(t *testing.T) {
	p := testcred.Secret("weak")
	enc, err := Hash(p, salt())
	if err != nil {
		t.Fatal(err)
	}
	weaker := strings.Replace(enc, "m=65536,t=3,p=4", "m=65536,t=1,p=4", 1)
	if ok, _, _ := Verify(weaker, p); ok {
		t.Fatal("changing the cost without rehashing still matched")
	}
	// Rebuild a genuinely weaker hash the way an older release would have.
	key := argonKey(p, salt(), 1, 32*1024, 2)
	old := "argon2id$v=19$m=32768,t=1,p=2$" + b64(salt()) + "$" + b64(key)
	if ok, stale, err := Verify(old, p); !ok || !stale || err != nil {
		t.Fatalf("Verify(weaker argon2id) = %v %v %v", ok, stale, err)
	}
}

func TestAMalformedHashIsAnErrorNotAMatch(t *testing.T) {
	p := testcred.Secret("bad")
	for _, enc := range []string{"", "argon2id$1$a$b", LegacyAlgorithm + "$x$a$b", LegacyAlgorithm + "$1000$!!$!!",
		LegacyAlgorithm + "$99999999$AAAAAAAAAAAAAAAAAAAAAA$AAAA", "argon2id$v=18$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAA",
		"argon2id$v=19$m=1,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAA", "argon2id$v=19$m=65536,t=3,p=4$!!$!!", "scrypt$1$2$3"} {
		if ok, _, err := Verify(enc, p); ok || err == nil {
			t.Errorf("Verify(%q) = %v, %v", enc, ok, err)
		}
	}
}

func TestPolicy(t *testing.T) {
	for _, bad := range []string{"", "12345", "123456", "654321", "aaaaaaa", "abcdefg", " " + testcred.Secret("sp"), "ab\x01cdefg", strings.Repeat("x7", 65)} {
		if Policy(bad) == nil {
			t.Errorf("Policy(%q) passed", bad)
		}
	}
	for _, good := range []string{"193746", testcred.Secret("good"), "correct horse battery"} {
		if err := Policy(good); err != nil {
			t.Errorf("Policy(%q) = %v", good, err)
		}
	}
}
