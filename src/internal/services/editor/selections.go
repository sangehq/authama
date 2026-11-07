package editor

import (
	"encoding/json"
	"fmt"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/util/log"
)

// SetSelection keeps raw as the manifest's saved export selection name
// (defaults.export.selections), or removes it when raw is nil. Removing one
// the manifest does not have is an error, so a typo is not a silent no-op.
func SetSelection(e *Editor, name string, raw json.RawMessage) error {
	m, err := e.load()
	if err != nil {
		return err
	}
	if raw == nil {
		found := false
		for _, n := range manifest.SelectionNames(m) {
			found = found || n == name
		}
		if !found {
			return fmt.Errorf("the manifest has no saved selection %q", name)
		}
	}
	manifest.SetSelection(m, name, raw)
	if err := e.save(m); err != nil {
		return err
	}
	action := "export.selection.save"
	if raw == nil {
		action = "export.selection.delete"
	}
	log.Audit(e.p.AuditLog(), action, log.Field{Key: "name", Value: name})
	return nil
}
