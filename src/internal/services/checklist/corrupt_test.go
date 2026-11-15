package checklist

import (
	"os"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
)

// A revoked.json or retired.json that does not parse is an error, not an empty
// record: read as empty, a revoked key was listed as in use, with nothing left
// to do (review R-BUG-05).
func TestAnUnreadableRevocationOrRetirementRecordIsAnError(t *testing.T) {
	for name, path := range map[string]func(string) string{
		"revoked.json": inventory.RevokedPath,
		"retired.json": inventory.RetiredPath,
	} {
		t.Run(name, func(t *testing.T) {
			h := fixture(t)
			if err := os.WriteFile(path(h.svc.P.Inventory()), []byte(`{"truncated`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := h.svc.Get("personal/personal_gh-ed25519"); err == nil {
				t.Errorf("a corrupt %s was read as no records", name)
			}
		})
	}
}
