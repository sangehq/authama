package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sangehq/authama/src/internal/util/fs"
)

// Revocation is one revoked key: the key it was, when it was revoked, and the
// targets it was removed from and could not be removed from. A fingerprint in
// the record is never deployed again; a regenerated or rotated key has a new
// fingerprint and is not affected.
type Revocation struct {
	Key        string   `json:"key"` // profile/key_name
	RevokedOn  string   `json:"revoked_on"`
	Reason     string   `json:"reason,omitempty"`
	Removed    []string `json:"removed_from"`
	NotRemoved []string `json:"not_removed_from"`
}

// RevokedPath is revoked.json, beside the inventory at inventoryPath. A file of
// its own, not a field: the inventory decodes strictly, so a new field would
// make an older authama refuse the whole inventory.
func RevokedPath(inventoryPath string) string {
	return filepath.Join(filepath.Dir(inventoryPath), "revoked.json")
}

// Revocations is revoked.json: revocations by fingerprint.
type Revocations map[string]Revocation

// LoadRevocations reads path; a missing file is no revocations.
func LoadRevocations(path string) (Revocations, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Revocations{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("revocations could not be read: %s: %w", path, err)
	}
	var r Revocations
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("revocations failed validation: %s: %w", path, err)
	}
	if r == nil {
		r = Revocations{}
	}
	return r, nil
}

// Save writes the record owner-only, keys sorted (json sorts map keys).
func (r Revocations) Save(path string) error {
	for fp, v := range r {
		sort.Strings(v.Removed)
		sort.Strings(v.NotRemoved)
		if v.Removed == nil {
			v.Removed = []string{}
		}
		if v.NotRemoved == nil {
			v.NotRemoved = []string{}
		}
		r[fp] = v
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return fs.WriteTextAtomic(path, string(b)+"\n", 0o600)
}
