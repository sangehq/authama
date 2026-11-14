package editor

import (
	"fmt"

	"github.com/sangehq/authama/src/internal/core/manifest"
)

// ReserveKey records that ~/.ssh/<name> is kept on purpose. It only edits the
// manifest; the key file is never touched.
func (e *Editor) ReserveKey(name string, r manifest.ReservedKey) error {
	if err := manifest.ValidReservedKeyName(name); err != nil {
		return err
	}
	m, err := e.load()
	if err != nil {
		return err
	}
	manifest.Reserve(m, name, r)
	return e.save(m)
}

// UnreserveKey drops a reservation. missingOK makes an absent one a no-op
// (adopting a key releases its reservation whether or not it had one).
func (e *Editor) UnreserveKey(name string, missingOK bool) (bool, error) {
	m, err := e.load()
	if err != nil {
		return false, err
	}
	if !manifest.Unreserve(m, name) {
		if missingOK {
			return false, nil
		}
		return false, fmt.Errorf("%s is not a reserved key (see `authama ssh key list`)", name)
	}
	return true, e.save(m)
}
