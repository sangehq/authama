package pin

import (
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/testcred"
)

func salt() []byte { return []byte(testcred.Secret("salt"))[:SaltLen] }

func TestAHashChecksTheRightPINAndNoOther(t *testing.T) {
	p := testcred.Secret("owner")
	enc, err := Hash(p, salt())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, Algorithm+"$600000$") || strings.Contains(enc, p) {
		t.Fatalf("encoded form: %s", enc)
	}
	if ok, stale, err := Verify(enc, p); !ok || stale || err != nil {
		t.Fatalf("Verify(right) = %v %v %v", ok, stale, err)
	}
	if ok, _, _ := Verify(enc, p+"x"); ok {
		t.Fatal("a different PIN verified")
	}
}

// A hash made at a lower cost still verifies, and says it should be redone.
func TestAWeakerHashVerifiesAndIsStale(t *testing.T) {
	p := testcred.Secret("weak")
	enc, err := encode(1000, salt(), p)
	if err != nil {
		t.Fatal(err)
	}
	if ok, stale, err := Verify(enc, p); !ok || !stale || err != nil {
		t.Fatalf("Verify = %v %v %v", ok, stale, err)
	}
}

func TestAMalformedHashIsAnErrorNotAMatch(t *testing.T) {
	p := testcred.Secret("bad")
	for _, enc := range []string{"", "argon2id$1$a$b", Algorithm + "$x$a$b", Algorithm + "$1000$!!$!!", Algorithm + "$99999999$AAAAAAAAAAAAAAAAAAAAAA$AAAA"} {
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
