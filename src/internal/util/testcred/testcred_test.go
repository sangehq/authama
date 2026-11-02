package testcred

import (
	"strings"
	"testing"
)

func TestValuesAreStablePerPurposeAndDistinctAcrossPurposes(t *testing.T) {
	a, b := Secret("alpha"), Secret("beta")
	if a != Secret("alpha") {
		t.Error("a purpose must keep its value for the whole run")
	}
	if a == b {
		t.Error("two purposes must not share a value")
	}
	if len(a) != 32 {
		t.Errorf("len = %d, want 32", len(a))
	}
	if tok := Token("ghp", "alpha"); tok != "ghp_"+a || !strings.HasPrefix(tok, "ghp_") {
		t.Errorf("Token = %q", tok)
	}
}
