package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sangehq/authama/src/internal/util/fs"
)

// Retirement is one key a rotation replaced: what it was, its successor, when
// it was retired, and the targets the rotation removed it from, could not
// remove it from, and that take it off by hand (with where). It is not a
// revocation: rollback may still bring a retired key back, while deploy and
// rollback refuse a revoked one.
type Retirement struct {
	Key        string            `json:"key"` // profile/key_name
	Successor  string            `json:"successor"`
	RetiredOn  string            `json:"retired_on"`
	Removed    []string          `json:"removed_from"`
	NotRemoved []string          `json:"not_removed_from"`
	ByHand     map[string]string `json:"by_hand,omitempty"` // target -> where its keys are managed
}

// RetiredPath is retired.json, beside the inventory at inventoryPath. A file
// of its own for the reason revoked.json is one: the inventory decodes
// strictly, so a new field would make an older authama refuse it.
func RetiredPath(inventoryPath string) string {
	return filepath.Join(filepath.Dir(inventoryPath), "retired.json")
}

// Retirements is retired.json: retirements by the retired key's fingerprint.
type Retirements map[string]Retirement

// LoadRetirements reads path; a missing file is no retirements.
func LoadRetirements(path string) (Retirements, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Retirements{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("retirements could not be read: %s: %w", path, err)
	}
	var r Retirements
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("retirements failed validation: %s: %w", path, err)
	}
	if r == nil {
		r = Retirements{}
	}
	return r, nil
}

// Save writes the record owner-only, lists sorted.
func (r Retirements) Save(path string) error {
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
