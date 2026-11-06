package tokensvc

import (
	"strings"
	"testing"
)

// A Composer service names its tool, so another tool beside it is refused
// rather than one of the two being taken silently; composer beside it is the
// same tool said twice, and fine.
func TestAServiceBesideAnotherToolIsRefused(t *testing.T) {
	_, err := SetSpec{Service: "github", Target: "npm", Server: "github.com"}.Check()
	if err == nil || !strings.Contains(err.Error(), "--service is for Composer") {
		t.Fatalf("service github with target npm: err = %v, want the contradiction refused", err)
	}
	pl, err := SetSpec{Service: "github", Target: "composer", Server: "github.com"}.Check()
	if err != nil || pl.Target != "composer" {
		t.Fatalf("service github with target composer = %+v, %v", pl, err)
	}
	pl, err = SetSpec{Service: "github", Server: "github.com"}.Check()
	if err != nil || pl.Target != "composer" {
		t.Fatalf("service github alone = %+v, %v", pl, err)
	}
}
